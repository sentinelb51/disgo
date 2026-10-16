package gateway

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
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

// UnmarshalJSONFrom decodes the frame as it streams in. Handed to UnmarshalJSON instead, the frame is read in full
// by the decoder and then again by UnmarshalJSON; here only d is read twice, once to keep it as RawD
func (e *Message) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var v rawMessage
	if err := jsonv2.UnmarshalDecode(dec, &v); err != nil {
		return err
	}
	return e.unmarshal(v, v.D)
}

func (e *Message) unmarshal(v rawMessage, data []byte) error {
	var (
		messageData MessageData
		err         error
	)

	switch v.Op {
	case OpcodeDispatch:
		messageData, err = UnmarshalEventData(v.D, v.T)

	case OpcodeHeartbeat:
		var d MessageDataHeartbeat
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeIdentify:
		var d MessageDataIdentify
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodePresenceUpdate:
		var d MessageDataPresenceUpdate
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeVoiceStateUpdate:
		var d MessageDataVoiceStateUpdate
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeResume:
		var d MessageDataResume
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeReconnect:
		messageData = MessageDataReconnect{}

	case OpcodeRequestGuildMembers:
		var d MessageDataRequestGuildMembers
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeInvalidSession:
		var d MessageDataInvalidSession
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeHello:
		var d MessageDataHello
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeHeartbeatACK:
		messageData = MessageDataHeartbeatACK{}

	case OpcodeRequestSoundboardSounds:
		var d MessageDataRequestSoundboardSounds
		err = unmarshalData(v.D, &d)
		messageData = d

	case OpcodeRequestChannelInfo:
		var d MessageDataRequestChannelInfo
		err = unmarshalData(v.D, &d)
		messageData = d

	default:
		var d MessageDataUnknown
		err = unmarshalData(v.D, &d)
		messageData = d
	}
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
	var (
		eventData EventData
		err       error
	)
	switch eventType {
	case EventTypeReady:
		var d EventReady
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeResumed:
		eventData = EventResumed{}

	case EventTypeRateLimited:
		var d EventRateLimited
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeApplicationCommandPermissionsUpdate:
		var d EventApplicationCommandPermissionsUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeAutoModerationRuleCreate:
		var d EventAutoModerationRuleCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeAutoModerationRuleUpdate:
		var d EventAutoModerationRuleUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeAutoModerationRuleDelete:
		var d EventAutoModerationRuleDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeAutoModerationActionExecution:
		var d EventAutoModerationActionExecution
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeChannelCreate:
		var d EventChannelCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeChannelUpdate:
		var d EventChannelUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeChannelDelete:
		var d EventChannelDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeChannelInfo:
		var d EventChannelInfo
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeChannelPinsUpdate:
		var d EventChannelPinsUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeEntitlementCreate:
		var d EventEntitlementCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeEntitlementUpdate:
		var d EventEntitlementUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeEntitlementDelete:
		var d EventEntitlementDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeThreadCreate:
		var d EventThreadCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeThreadUpdate:
		var d EventThreadUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeThreadDelete:
		var d EventThreadDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeThreadListSync:
		var d EventThreadListSync
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeThreadMemberUpdate:
		var d EventThreadMemberUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeThreadMembersUpdate:
		var d EventThreadMembersUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildCreate:
		var d EventGuildCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildUpdate:
		var d EventGuildUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildDelete:
		var d EventGuildDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildAuditLogEntryCreate:
		var d EventGuildAuditLogEntryCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildBanAdd:
		var d EventGuildBanAdd
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildBanRemove:
		var d EventGuildBanRemove
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildEmojisUpdate:
		var d EventGuildEmojisUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildStickersUpdate:
		var d EventGuildStickersUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildIntegrationsUpdate:
		var d EventGuildIntegrationsUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildMemberAdd:
		var d EventGuildMemberAdd
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildMemberRemove:
		var d EventGuildMemberRemove
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildMemberUpdate:
		var d EventGuildMemberUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildMembersChunk:
		var d EventGuildMembersChunk
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildRoleCreate:
		var d EventGuildRoleCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildRoleUpdate:
		var d EventGuildRoleUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildRoleDelete:
		var d EventGuildRoleDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildScheduledEventCreate:
		var d EventGuildScheduledEventCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildScheduledEventUpdate:
		var d EventGuildScheduledEventUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildScheduledEventDelete:
		var d EventGuildScheduledEventDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildScheduledEventUserAdd:
		var d EventGuildScheduledEventUserAdd
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildScheduledEventUserRemove:
		var d EventGuildScheduledEventUserRemove
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildSoundboardSoundCreate:
		var d EventGuildSoundboardSoundCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildSoundboardSoundUpdate:
		var d EventGuildSoundboardSoundUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildSoundboardSoundDelete:
		var d EventGuildSoundboardSoundDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeGuildSoundboardSoundsUpdate:
		var d EventGuildSoundboardSoundsUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeIntegrationCreate:
		var d EventIntegrationCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeIntegrationUpdate:
		var d EventIntegrationUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeIntegrationDelete:
		var d EventIntegrationDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeInteractionCreate:
		// Called directly: json.Unmarshal would read the whole payload once before handing it over
		var d EventInteractionCreate
		err = d.UnmarshalJSON(data)
		eventData = d

	case EventTypeInviteCreate:
		var d EventInviteCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeInviteDelete:
		var d EventInviteDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageCreate:
		var d EventMessageCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageUpdate:
		var d EventMessageUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageDelete:
		var d EventMessageDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageDeleteBulk:
		var d EventMessageDeleteBulk
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageReactionAdd:
		var d EventMessageReactionAdd
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageReactionRemove:
		var d EventMessageReactionRemove
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageReactionRemoveAll:
		var d EventMessageReactionRemoveAll
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeMessageReactionRemoveEmoji:
		var d EventMessageReactionRemoveEmoji
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypePresenceUpdate:
		var d EventPresenceUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeSoundboardSounds:
		var d EventSoundboardSounds
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeStageInstanceCreate:
		var d EventStageInstanceCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeStageInstanceUpdate:
		var d EventStageInstanceUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeStageInstanceDelete:
		var d EventStageInstanceDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeSubscriptionCreate:
		var d EventSubscriptionCreate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeSubscriptionUpdate:
		var d EventSubscriptionUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeSubscriptionDelete:
		var d EventSubscriptionDelete
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeTypingStart:
		var d EventTypingStart
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeUserUpdate:
		var d EventUserUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeVoiceChannelEffectSend:
		var d EventVoiceChannelEffectSend
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeVoiceChannelStatusUpdate:
		var d EventVoiceChannelStatusUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeVoiceChannelStartTimeUpdate:
		var d EventVoiceChannelStartTimeUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeVoiceStateUpdate:
		var d EventVoiceStateUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeVoiceServerUpdate:
		var d EventVoiceServerUpdate
		err = unmarshalData(data, &d)
		eventData = d

	case EventTypeWebhooksUpdate:
		var d EventWebhooksUpdate
		err = unmarshalData(data, &d)
		eventData = d

	default:
		var d EventUnknown
		err = unmarshalData(data, &d)
		eventData = d
	}

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data: %s: %w", string(data), err)
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
