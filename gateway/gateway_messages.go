package gateway

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/internal/flags"
)

// Message raw Message type
type Message struct {
	Op   Opcode          `json:"op"`
	S    int             `json:"s,omitempty"`
	T    EventType       `json:"t,omitempty"`
	D    MessageData     `json:"d,omitempty"`
	RawD json.RawMessage `json:"-"`
}

type rawMessage struct {
	Op Opcode          `json:"op"`
	S  int             `json:"s,omitempty"`
	T  EventType       `json:"t,omitempty"`
	D  json.RawMessage `json:"d,omitempty"`
}

func (e *Message) UnmarshalJSON(data []byte) error {
	var v rawMessage
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return e.unmarshal(v, data)
}

// UnmarshalJSONFrom decodes the frame as it streams in, keeping d as RawD. Handed to UnmarshalJSON instead, the
// frame is read in full by the decoder and then again by UnmarshalJSON
func (e *Message) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	return e.decode(dec, true)
}

// decode reads a frame from dec. Discord sends op and t before d, so d is decoded into its event as it streams in,
// read once, unless keepRaw: then d is copied out as RawD first and decoded from there, a second read. d arriving
// before op, or before t in a dispatch, is kept and decoded after the frame too, as it must be
func (e *Message) decode(dec *jsontext.Decoder, keepRaw bool) error {
	if kind := dec.PeekKind(); kind != '{' {
		// As the struct decoder words it, and reads past it
		var v rawMessage
		return jsonv2.UnmarshalDecode(dec, &v)
	}
	if _, err := dec.ReadToken(); err != nil {
		return err
	}

	var (
		v                    rawMessage
		data                 MessageData
		hasOp, hasT, decoded bool
	)
	for dec.PeekKind() == '"' {
		name, err := dec.ReadValue()
		if err != nil {
			return err
		}

		switch string(name) {
		case `"op"`:
			err = jsonv2.UnmarshalDecode(dec, &v.Op)
			hasOp = true
		case `"s"`:
			err = jsonv2.UnmarshalDecode(dec, &v.S)
		case `"t"`:
			err = jsonv2.UnmarshalDecode(dec, &v.T)
			hasT = true
		case `"d"`:
			if keepRaw || !hasOp || (v.Op == OpcodeDispatch && !hasT) {
				err = jsonv2.UnmarshalDecode(dec, &v.D)
				break
			}

			data, err = decodeMessageData(v.Op, v.T, func(d any) error {
				if d == nil {
					return dec.SkipValue()
				}
				return jsonv2.UnmarshalDecode(dec, d)
			})
			if err != nil && !isFrameError(err) {
				return &messageDataError{err: fmt.Errorf("failed to unmarshal message data: op %d %s: %w", v.Op, v.T, err)}
			}
			decoded = true
		default:
			err = dec.SkipValue()
		}
		if err != nil {
			return err
		}
	}
	if _, err := dec.ReadToken(); err != nil {
		return err
	}

	if !decoded {
		return e.unmarshal(v, v.D)
	}

	*e = Message{Op: v.Op, S: v.S, T: v.T, D: data}
	return nil
}

// isFrameError reports whether err leaves the frame unreadable (malformed, cut short, unreadable), rather than its
// data not fitting its event, which encoding/json/v2 reports as a SemanticError
func isFrameError(err error) bool {
	var (
		syntactic *jsontext.SyntacticError
		semantic  *jsonv2.SemanticError
	)
	return errors.As(err, &syntactic) || errors.Is(err, io.ErrUnexpectedEOF) || !errors.As(err, &semantic)
}

func (e *Message) unmarshal(v rawMessage, data []byte) error {
	messageData, err := decodeMessageData(v.Op, v.T, func(d any) error {
		if d == nil {
			return nil
		}
		return unmarshalData(v.D, d)
	})
	if err != nil {
		return &messageDataError{err: fmt.Errorf("failed to unmarshal message data: %s: %w", string(data), err)}
	}
	e.Op = v.Op
	e.S = v.S
	e.T = v.T
	e.D = messageData
	e.RawD = v.D
	return nil
}

// decodeMessageData decodes the data of a frame of op (and t, for a dispatch) through decode, as decodeEventData does
func decodeMessageData(op Opcode, t EventType, decode func(v any) error) (MessageData, error) {
	var (
		messageData MessageData
		err         error
	)

	switch op {
	case OpcodeDispatch:
		messageData, err = decodeEventData(t, decode)

	case OpcodeHeartbeat:
		var d MessageDataHeartbeat
		err = decode(&d)
		messageData = d

	case OpcodeIdentify:
		var d MessageDataIdentify
		err = decode(&d)
		messageData = d

	case OpcodePresenceUpdate:
		var d MessageDataPresenceUpdate
		err = decode(&d)
		messageData = d

	case OpcodeVoiceStateUpdate:
		var d MessageDataVoiceStateUpdate
		err = decode(&d)
		messageData = d

	case OpcodeResume:
		var d MessageDataResume
		err = decode(&d)
		messageData = d

	case OpcodeReconnect:
		err = decode(nil)
		messageData = MessageDataReconnect{}

	case OpcodeRequestGuildMembers:
		var d MessageDataRequestGuildMembers
		err = decode(&d)
		messageData = d

	case OpcodeInvalidSession:
		var d MessageDataInvalidSession
		err = decode(&d)
		messageData = d

	case OpcodeHello:
		var d MessageDataHello
		err = decode(&d)
		messageData = d

	case OpcodeHeartbeatACK:
		err = decode(nil)
		messageData = MessageDataHeartbeatACK{}

	case OpcodeRequestSoundboardSounds:
		var d MessageDataRequestSoundboardSounds
		err = decode(&d)
		messageData = d

	case OpcodeRequestChannelInfo:
		var d MessageDataRequestChannelInfo
		err = decode(&d)
		messageData = d

	default:
		var d MessageDataUnknown
		err = decode(&d)
		messageData = d
	}

	return messageData, err
}

// messageDataError is a Message whose frame was read whole, but whose data did not decode
type messageDataError struct {
	err error
}

func (e *messageDataError) Error() string { return e.err.Error() }
func (e *messageDataError) Unwrap() error { return e.err }

// unmarshalData decodes a frame's data. json.Unmarshal would first read it all to validate it, which the frame's
// decoder has already done
func unmarshalData(data []byte, v any) error {
	return jsonv2.Unmarshal(data, v, decodeOptions)
}

// MessageData is the interface for all message data types
type MessageData interface {
	messageData()
}

func UnmarshalEventData(data []byte, eventType EventType) (EventData, error) {
	eventData, err := decodeEventData(eventType, func(v any) error {
		if v == nil {
			return nil
		}
		return unmarshalData(data, v)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data: %s: %w", string(data), err)
	}

	return eventData, nil
}

// decodeEventData decodes the data of an event of eventType through decode, which reads one JSON value into the
// pointer it is handed, or past it for nil
func decodeEventData(eventType EventType, decode func(v any) error) (EventData, error) {
	var (
		eventData EventData
		err       error
	)
	switch eventType {
	case EventTypeReady:
		var d EventReady
		err = decode(&d)
		eventData = d

	case EventTypeResumed:
		err = decode(nil)
		eventData = EventResumed{}

	case EventTypeRateLimited:
		var d EventRateLimited
		err = decode(&d)
		eventData = d

	case EventTypeApplicationCommandPermissionsUpdate:
		var d EventApplicationCommandPermissionsUpdate
		err = decode(&d)
		eventData = d

	case EventTypeAutoModerationRuleCreate:
		var d EventAutoModerationRuleCreate
		err = decode(&d)
		eventData = d

	case EventTypeAutoModerationRuleUpdate:
		var d EventAutoModerationRuleUpdate
		err = decode(&d)
		eventData = d

	case EventTypeAutoModerationRuleDelete:
		var d EventAutoModerationRuleDelete
		err = decode(&d)
		eventData = d

	case EventTypeAutoModerationActionExecution:
		var d EventAutoModerationActionExecution
		err = decode(&d)
		eventData = d

	case EventTypeChannelCreate:
		var d EventChannelCreate
		err = decode(&d)
		eventData = d

	case EventTypeChannelUpdate:
		var d EventChannelUpdate
		err = decode(&d)
		eventData = d

	case EventTypeChannelDelete:
		var d EventChannelDelete
		err = decode(&d)
		eventData = d

	case EventTypeChannelInfo:
		var d EventChannelInfo
		err = decode(&d)
		eventData = d

	case EventTypeChannelPinsUpdate:
		var d EventChannelPinsUpdate
		err = decode(&d)
		eventData = d

	case EventTypeEntitlementCreate:
		var d EventEntitlementCreate
		err = decode(&d)
		eventData = d

	case EventTypeEntitlementUpdate:
		var d EventEntitlementUpdate
		err = decode(&d)
		eventData = d

	case EventTypeEntitlementDelete:
		var d EventEntitlementDelete
		err = decode(&d)
		eventData = d

	case EventTypeThreadCreate:
		var d EventThreadCreate
		err = decode(&d)
		eventData = d

	case EventTypeThreadUpdate:
		var d EventThreadUpdate
		err = decode(&d)
		eventData = d

	case EventTypeThreadDelete:
		var d EventThreadDelete
		err = decode(&d)
		eventData = d

	case EventTypeThreadListSync:
		var d EventThreadListSync
		err = decode(&d)
		eventData = d

	case EventTypeThreadMemberUpdate:
		var d EventThreadMemberUpdate
		err = decode(&d)
		eventData = d

	case EventTypeThreadMembersUpdate:
		var d EventThreadMembersUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildCreate:
		var d EventGuildCreate
		err = decode(&d)
		eventData = d

	case EventTypeGuildUpdate:
		var d EventGuildUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildDelete:
		var d EventGuildDelete
		err = decode(&d)
		eventData = d

	case EventTypeGuildAuditLogEntryCreate:
		var d EventGuildAuditLogEntryCreate
		err = decode(&d)
		eventData = d

	case EventTypeGuildBanAdd:
		var d EventGuildBanAdd
		err = decode(&d)
		eventData = d

	case EventTypeGuildBanRemove:
		var d EventGuildBanRemove
		err = decode(&d)
		eventData = d

	case EventTypeGuildEmojisUpdate:
		var d EventGuildEmojisUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildStickersUpdate:
		var d EventGuildStickersUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildIntegrationsUpdate:
		var d EventGuildIntegrationsUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildMemberAdd:
		var d EventGuildMemberAdd
		err = decode(&d)
		eventData = d

	case EventTypeGuildMemberRemove:
		var d EventGuildMemberRemove
		err = decode(&d)
		eventData = d

	case EventTypeGuildMemberUpdate:
		var d EventGuildMemberUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildMembersChunk:
		var d EventGuildMembersChunk
		err = decode(&d)
		eventData = d

	case EventTypeGuildRoleCreate:
		var d EventGuildRoleCreate
		err = decode(&d)
		eventData = d

	case EventTypeGuildRoleUpdate:
		var d EventGuildRoleUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildRoleDelete:
		var d EventGuildRoleDelete
		err = decode(&d)
		eventData = d

	case EventTypeGuildScheduledEventCreate:
		var d EventGuildScheduledEventCreate
		err = decode(&d)
		eventData = d

	case EventTypeGuildScheduledEventUpdate:
		var d EventGuildScheduledEventUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildScheduledEventDelete:
		var d EventGuildScheduledEventDelete
		err = decode(&d)
		eventData = d

	case EventTypeGuildScheduledEventUserAdd:
		var d EventGuildScheduledEventUserAdd
		err = decode(&d)
		eventData = d

	case EventTypeGuildScheduledEventUserRemove:
		var d EventGuildScheduledEventUserRemove
		err = decode(&d)
		eventData = d

	case EventTypeGuildSoundboardSoundCreate:
		var d EventGuildSoundboardSoundCreate
		err = decode(&d)
		eventData = d

	case EventTypeGuildSoundboardSoundUpdate:
		var d EventGuildSoundboardSoundUpdate
		err = decode(&d)
		eventData = d

	case EventTypeGuildSoundboardSoundDelete:
		var d EventGuildSoundboardSoundDelete
		err = decode(&d)
		eventData = d

	case EventTypeGuildSoundboardSoundsUpdate:
		var d EventGuildSoundboardSoundsUpdate
		err = decode(&d)
		eventData = d

	case EventTypeIntegrationCreate:
		var d EventIntegrationCreate
		err = decode(&d)
		eventData = d

	case EventTypeIntegrationUpdate:
		var d EventIntegrationUpdate
		err = decode(&d)
		eventData = d

	case EventTypeIntegrationDelete:
		var d EventIntegrationDelete
		err = decode(&d)
		eventData = d

	case EventTypeInteractionCreate:
		var d EventInteractionCreate
		err = decode(&d)
		eventData = d

	case EventTypeInviteCreate:
		var d EventInviteCreate
		err = decode(&d)
		eventData = d

	case EventTypeInviteDelete:
		var d EventInviteDelete
		err = decode(&d)
		eventData = d

	case EventTypeMessageCreate:
		var d EventMessageCreate
		err = decode(&d)
		eventData = d

	case EventTypeMessageUpdate:
		var d EventMessageUpdate
		err = decode(&d)
		eventData = d

	case EventTypeMessageDelete:
		var d EventMessageDelete
		err = decode(&d)
		eventData = d

	case EventTypeMessageDeleteBulk:
		var d EventMessageDeleteBulk
		err = decode(&d)
		eventData = d

	case EventTypeMessagePollVoteAdd:
		var d EventMessagePollVoteAdd
		err = decode(&d)
		eventData = d

	case EventTypeMessagePollVoteRemove:
		var d EventMessagePollVoteRemove
		err = decode(&d)
		eventData = d

	case EventTypeMessageReactionAdd:
		var d EventMessageReactionAdd
		err = decode(&d)
		eventData = d

	case EventTypeMessageReactionRemove:
		var d EventMessageReactionRemove
		err = decode(&d)
		eventData = d

	case EventTypeMessageReactionRemoveAll:
		var d EventMessageReactionRemoveAll
		err = decode(&d)
		eventData = d

	case EventTypeMessageReactionRemoveEmoji:
		var d EventMessageReactionRemoveEmoji
		err = decode(&d)
		eventData = d

	case EventTypePresenceUpdate:
		var d EventPresenceUpdate
		err = decode(&d)
		eventData = d

	case EventTypeSoundboardSounds:
		var d EventSoundboardSounds
		err = decode(&d)
		eventData = d

	case EventTypeStageInstanceCreate:
		var d EventStageInstanceCreate
		err = decode(&d)
		eventData = d

	case EventTypeStageInstanceUpdate:
		var d EventStageInstanceUpdate
		err = decode(&d)
		eventData = d

	case EventTypeStageInstanceDelete:
		var d EventStageInstanceDelete
		err = decode(&d)
		eventData = d

	case EventTypeSubscriptionCreate:
		var d EventSubscriptionCreate
		err = decode(&d)
		eventData = d

	case EventTypeSubscriptionUpdate:
		var d EventSubscriptionUpdate
		err = decode(&d)
		eventData = d

	case EventTypeSubscriptionDelete:
		var d EventSubscriptionDelete
		err = decode(&d)
		eventData = d

	case EventTypeTypingStart:
		var d EventTypingStart
		err = decode(&d)
		eventData = d

	case EventTypeUserUpdate:
		var d EventUserUpdate
		err = decode(&d)
		eventData = d

	case EventTypeVoiceChannelEffectSend:
		var d EventVoiceChannelEffectSend
		err = decode(&d)
		eventData = d

	case EventTypeVoiceChannelStatusUpdate:
		var d EventVoiceChannelStatusUpdate
		err = decode(&d)
		eventData = d

	case EventTypeVoiceChannelStartTimeUpdate:
		var d EventVoiceChannelStartTimeUpdate
		err = decode(&d)
		eventData = d

	case EventTypeVoiceStateUpdate:
		var d EventVoiceStateUpdate
		err = decode(&d)
		eventData = d

	case EventTypeVoiceServerUpdate:
		var d EventVoiceServerUpdate
		err = decode(&d)
		eventData = d

	case EventTypeWebhooksUpdate:
		var d EventWebhooksUpdate
		err = decode(&d)
		eventData = d

	default:
		var d EventUnknown
		err = decode(&d)
		eventData = d
	}

	if err != nil {
		return nil, err
	}

	return eventData, nil
}

type MessageDataUnknown json.RawMessage

func (MessageDataUnknown) messageData() {}

// MessageDataHeartbeat is used to ensure the websocket connection remains open, and disconnect if not.
type MessageDataHeartbeat int

func (m MessageDataHeartbeat) MarshalJSON() ([]byte, error) {
	if m == 0 {
		return []byte("null"), nil
	}
	return []byte(strconv.Itoa(int(m))), nil
}

func (MessageDataHeartbeat) messageData() {}

// MessageDataIdentify is the data used in IdentifyCommandData
type MessageDataIdentify struct {
	Token          string                        `json:"token"`
	Properties     IdentifyCommandDataProperties `json:"properties"`
	Compress       bool                          `json:"compress,omitempty"`
	LargeThreshold int                           `json:"large_threshold,omitempty"`
	Shard          *[2]int                       `json:"shard,omitempty"`
	Intents        Intents                       `json:"intents"`
	Presence       *MessageDataPresenceUpdate    `json:"presence,omitempty"`
	Capabilities   GatewayCapabilities           `json:"capabilities,omitempty"`
}

func (MessageDataIdentify) messageData() {}

// GatewayCapabilities is a bitfield used to opt into in-progress gateway behaviors.
type GatewayCapabilities int

const (
	// GatewayCapabilityChannelObfuscation opts the app into receiving obfuscated channel metadata
	// over the Gateway for channels it can't view (temporary).
	GatewayCapabilityChannelObfuscation GatewayCapabilities = 1 << 15
	GatewayCapabilitiesNone             GatewayCapabilities = 0
)

// Add allows you to add multiple bits together, producing a new bit
func (c GatewayCapabilities) Add(bits ...GatewayCapabilities) GatewayCapabilities {
	return flags.Add(c, bits...)
}

// Remove allows you to subtract multiple bits from the first, producing a new bit
func (c GatewayCapabilities) Remove(bits ...GatewayCapabilities) GatewayCapabilities {
	return flags.Remove(c, bits...)
}

// Has will ensure that the bit includes all the bits entered
func (c GatewayCapabilities) Has(bits ...GatewayCapabilities) bool {
	return flags.Has(c, bits...)
}

// Missing will check whether the bit is missing any one of the bits
func (c GatewayCapabilities) Missing(bits ...GatewayCapabilities) bool {
	return flags.Missing(c, bits...)
}

// IdentifyCommandDataProperties is used for specifying to discord which library and OS the bot is using, is
// automatically handled by the library and should rarely be used.
type IdentifyCommandDataProperties struct {
	OS      string `json:"os"`      // user OS
	Browser string `json:"browser"` // library name
	Device  string `json:"device"`  // library name
}

// MessageDataPresenceUpdate is used for updating Client's presence
type MessageDataPresenceUpdate struct {
	Since      *int64               `json:"since"`
	Activities []discord.Activity   `json:"activities"`
	Status     discord.OnlineStatus `json:"status"`
	AFK        bool                 `json:"afk"`
}

func (MessageDataPresenceUpdate) messageData() {}

type PresenceOpt func(presenceUpdate *MessageDataPresenceUpdate)

// WithPlayingActivity creates a new "Playing ..." activity of type discord.ActivityTypeGame
func WithPlayingActivity(name string, opts ...ActivityOpt) PresenceOpt {
	return withActivity(discord.Activity{
		Name: name,
		Type: discord.ActivityTypeGame,
	}, opts...)
}

// WithStreamingActivity creates a new "Streaming ..." activity of type discord.ActivityTypeStreaming
func WithStreamingActivity(name string, url string, opts ...ActivityOpt) PresenceOpt {
	activity := discord.Activity{
		Name: name,
		Type: discord.ActivityTypeStreaming,
	}
	if url != "" {
		activity.URL = &url
	}
	return withActivity(activity, opts...)
}

// WithListeningActivity creates a new "Listening to ..." activity of type discord.ActivityTypeListening
func WithListeningActivity(name string, opts ...ActivityOpt) PresenceOpt {
	return withActivity(discord.Activity{
		Name: name,
		Type: discord.ActivityTypeListening,
	}, opts...)
}

// WithWatchingActivity creates a new "Watching ..." activity of type discord.ActivityTypeWatching
func WithWatchingActivity(name string, opts ...ActivityOpt) PresenceOpt {
	return withActivity(discord.Activity{
		Name: name,
		Type: discord.ActivityTypeWatching,
	}, opts...)
}

// WithCustomActivity creates a new activity of type discord.ActivityTypeCustom
func WithCustomActivity(status string, opts ...ActivityOpt) PresenceOpt {
	return withActivity(discord.Activity{
		Name:  "Custom Status",
		Type:  discord.ActivityTypeCustom,
		State: &status,
	}, opts...)
}

// WithCompetingActivity creates a new "Competing in ..." activity of type discord.ActivityTypeCompeting
func WithCompetingActivity(name string, opts ...ActivityOpt) PresenceOpt {
	return withActivity(discord.Activity{
		Name: name,
		Type: discord.ActivityTypeCompeting,
	}, opts...)
}

func withActivity(activity discord.Activity, opts ...ActivityOpt) PresenceOpt {
	return func(presence *MessageDataPresenceUpdate) {
		for _, opt := range opts {
			opt(&activity)
		}
		presence.Activities = []discord.Activity{activity}
	}
}

// WithOnlineStatus sets the online status to the provided discord.OnlineStatus
func WithOnlineStatus(status discord.OnlineStatus) PresenceOpt {
	return func(presence *MessageDataPresenceUpdate) {
		presence.Status = status
	}
}

// WithAfk sets whether the session is afk
func WithAfk(afk bool) PresenceOpt {
	return func(presence *MessageDataPresenceUpdate) {
		presence.AFK = afk
	}
}

// WithSince sets when the session has gone afk
func WithSince(since *int64) PresenceOpt {
	return func(presence *MessageDataPresenceUpdate) {
		presence.Since = since
	}
}

// ActivityOpt is a type alias for a function that sets optional data for an Activity
type ActivityOpt func(activity *discord.Activity)

// WithActivityState sets the Activity.State
func WithActivityState(state string) ActivityOpt {
	return func(activity *discord.Activity) {
		activity.State = &state
	}
}

// MessageDataVoiceStateUpdate is used for updating the bots voice state in a guild
type MessageDataVoiceStateUpdate struct {
	GuildID   snowflake.ID  `json:"guild_id"`
	ChannelID *snowflake.ID `json:"channel_id"`
	SelfMute  bool          `json:"self_mute"`
	SelfDeaf  bool          `json:"self_deaf"`
}

func (MessageDataVoiceStateUpdate) messageData() {}

// MessageDataResume is used to resume a connection to discord in the case that you are disconnected. Is automatically
// handled by the library and should rarely be used.
type MessageDataResume struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int    `json:"seq"`
}

func (MessageDataResume) messageData() {}

type MessageDataReconnect struct{}

func (MessageDataReconnect) messageData() {}

// MessageDataRequestGuildMembers is used for fetching all the members of a guild_events. It is recommended you have a strict
// member caching policy when using this.
type MessageDataRequestGuildMembers struct {
	GuildID   snowflake.ID   `json:"guild_id"`
	Query     *string        `json:"query,omitempty"` // If specified, user_ids must not be entered
	Limit     *int           `json:"limit,omitempty"` // Must be >=1 if query/user_ids is used, otherwise 0
	Presences bool           `json:"presences,omitempty"`
	UserIDs   []snowflake.ID `json:"user_ids,omitempty"` // If specified, query must not be entered
	Nonce     string         `json:"nonce,omitempty"`    // All responses are hashed with this nonce, optional
}

func (MessageDataRequestGuildMembers) messageData() {}

type MessageDataInvalidSession bool

func (MessageDataInvalidSession) messageData() {}

type MessageDataHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

func (MessageDataHello) messageData() {}

type MessageDataHeartbeatACK struct{}

func (MessageDataHeartbeatACK) messageData() {}

type MessageDataRequestSoundboardSounds struct {
	GuildIDs []snowflake.ID `json:"guild_ids"`
}

func (MessageDataRequestSoundboardSounds) messageData() {}

// MessageDataRequestChannelInfo is used to request ephemeral channel data (opcode 43).
// The server will respond with an EventChannelInfo event containing the requested fields.
// Available fields: "status", "voice_start_time".
type MessageDataRequestChannelInfo struct {
	GuildID snowflake.ID `json:"guild_id"`
	Fields  []string     `json:"fields"`
}

func (MessageDataRequestChannelInfo) messageData() {}
