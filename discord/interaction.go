package discord

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"time"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"
)

// InteractionType is the type of Interaction
type InteractionType int

// Supported InteractionType(s)
const (
	InteractionTypePing InteractionType = iota + 1
	InteractionTypeApplicationCommand
	InteractionTypeComponent
	InteractionTypeAutocomplete
	InteractionTypeModalSubmit
)

type InteractionContextType int

const (
	InteractionContextTypeGuild InteractionContextType = iota
	InteractionContextTypeBotDM
	InteractionContextTypePrivateChannel
)

type rawInteraction struct {
	ID                           snowflake.ID                                `json:"id"`
	Type                         InteractionType                             `json:"type"`
	ApplicationID                snowflake.ID                                `json:"application_id"`
	Token                        string                                      `json:"token"`
	Version                      int                                         `json:"version"`
	Guild                        *InteractionGuild                           `json:"guild,omitempty"`
	GuildID                      *snowflake.ID                               `json:"guild_id,omitempty"`
	Channel                      InteractionChannel                          `json:"channel,omitempty"`
	Locale                       Locale                                      `json:"locale,omitempty"`
	GuildLocale                  *Locale                                     `json:"guild_locale,omitempty"`
	Member                       *ResolvedMember                             `json:"member,omitempty"`
	User                         *User                                       `json:"user,omitempty"`
	AppPermissions               *Permissions                                `json:"app_permissions,omitempty"`
	Entitlements                 []Entitlement                               `json:"entitlements"`
	AuthorizingIntegrationOwners map[ApplicationIntegrationType]snowflake.ID `json:"authorizing_integration_owners"`
	Context                      InteractionContextType                      `json:"context"`
	AttachmentSizeLimit          int                                         `json:"attachment_size_limit"`
}

// Interaction is used for easier unmarshalling of different Interaction(s)
type Interaction interface {
	Type() InteractionType
	ID() snowflake.ID
	ApplicationID() snowflake.ID
	Token() string
	Version() int
	PartialGuild() *InteractionGuild
	GuildID() *snowflake.ID
	Channel() InteractionChannel
	Locale() Locale
	GuildLocale() *Locale
	Member() *ResolvedMember
	User() User
	AppPermissions() *Permissions
	Entitlements() []Entitlement
	AuthorizingIntegrationOwners() map[ApplicationIntegrationType]snowflake.ID
	Context() InteractionContextType
	AttachmentSizeLimit() int
	CreatedAt() time.Time

	interaction()
}

func UnmarshalInteraction(data []byte) (Interaction, error) {
	// The type is peeked rather than probed, and the interaction decoded as it in one pass
	iType, _ := peek(data, "type", "")
	v := typedInteraction{interactionType: InteractionType(peekInt(iType))}
	if err := unmarshalOnce(data, &v); err != nil {
		return nil, err
	}

	return v.interaction, nil
}

// DecodeInteraction decodes the interaction dec reads next as its type as it streams in, the type peeked in what
// dec has buffered. One not buffered that far is read whole and peeked there
func DecodeInteraction(dec *jsontext.Decoder) (Interaction, error) {
	iType, _, ok := peekUnread(dec, "type", "")
	if !ok {
		data, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		return UnmarshalInteraction(data)
	}

	return decodeInteraction(InteractionType(peekInt(iType)), dec)
}

// typedInteraction decodes an interaction of a type known beforehand
type typedInteraction struct {
	interactionType InteractionType
	interaction     Interaction
}

func (i *typedInteraction) UnmarshalJSONFrom(dec *jsontext.Decoder) (err error) {
	i.interaction, err = decodeInteraction(i.interactionType, dec)
	return err
}

// decodeInteraction decodes the interaction dec reads next as interactionType, each kind's UnmarshalJSONFrom
// called directly
func decodeInteraction(interactionType InteractionType, dec *jsontext.Decoder) (Interaction, error) {
	var (
		interaction Interaction
		err         error
	)

	switch interactionType {
	case InteractionTypePing:
		v := PingInteraction{}
		err = v.UnmarshalJSONFrom(dec)
		interaction = v

	case InteractionTypeApplicationCommand:
		v := ApplicationCommandInteraction{}
		err = v.UnmarshalJSONFrom(dec)
		interaction = v

	case InteractionTypeComponent:
		v := ComponentInteraction{}
		err = v.UnmarshalJSONFrom(dec)
		interaction = v

	case InteractionTypeAutocomplete:
		v := AutocompleteInteraction{}
		err = v.UnmarshalJSONFrom(dec)
		interaction = v

	case InteractionTypeModalSubmit:
		v := ModalSubmitInteraction{}
		err = v.UnmarshalJSONFrom(dec)
		interaction = v

	default:
		if err = dec.SkipValue(); err == nil {
			err = fmt.Errorf("unknown rawInteraction with type %d received", interactionType)
		}
	}
	if err != nil {
		return nil, err
	}

	return interaction, nil
}

// decodeOptions are encoding/json's, but for its legacy errors, which validate a whole value before decoding it
var decodeOptions = jsonv2.JoinOptions(jsonv1.DefaultOptionsV1(), jsonv1.ReportErrorsWithLegacySemantics(false))

// unmarshalOnce decodes data in one pass where json.Unmarshal takes two, the first only to validate it. Errors
// are worded as encoding/json/v2 words them, and a malformed payload may leave v half filled; every caller
// returns the error and drops v
func unmarshalOnce(data []byte, v any) error {
	return jsonv2.Unmarshal(data, v, decodeOptions)
}

type ResolvedData struct {
	Users       map[snowflake.ID]User            `json:"users,omitempty"`
	Members     map[snowflake.ID]ResolvedMember  `json:"members,omitempty"`
	Roles       map[snowflake.ID]Role            `json:"roles,omitempty"`
	Channels    map[snowflake.ID]ResolvedChannel `json:"channels,omitempty"`
	Attachments map[snowflake.ID]Attachment      `json:"attachments,omitempty"`
}

func (r *ResolvedData) UnmarshalJSON(data []byte) error {
	return unmarshalOnce(data, r)
}

// resolvedData is ResolvedData without its methods, decoded by decodeResolvedData (decode_gen.go)
type resolvedData ResolvedData

// UnmarshalJSONFrom decodes the resolved data as it streams in, rather than read whole first
func (r *ResolvedData) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var v resolvedData
	if err := decodeResolvedData(dec, &v); err != nil {
		return err
	}
	*r = ResolvedData(v)
	for id, member := range r.Members {
		if user, ok := r.Users[id]; ok {
			member.User = user
			r.Members[id] = member
		}
	}
	return nil
}

type ResolvedMember struct {
	Member
	Permissions Permissions `json:"permissions,omitempty"`
}

func (ResolvedMember) isMentionableValue() {}

type ResolvedChannel struct {
	ID               snowflake.ID    `json:"id"`
	Name             string          `json:"name"`
	Type             ChannelType     `json:"type"`
	Permissions      Permissions     `json:"permissions"`
	AppPermissions   Permissions     `json:"app_permissions"`
	LastMessageID    *snowflake.ID   `json:"last_message_id,omitempty"`
	LastPinTimestamp *time.Time      `json:"last_pin_timestamp,omitempty"`
	NSFW             bool            `json:"nsfw,omitempty"`
	GuildID          snowflake.ID    `json:"guild_id"`
	Flags            ChannelFlags    `json:"flags,omitempty"`
	RateLimitPerUser int             `json:"rate_limit_per_user,omitempty"`
	Topic            *string         `json:"topic,omitempty"`
	Position         int             `json:"position,omitempty"`
	ThreadMetadata   *ThreadMetadata `json:"thread_metadata,omitempty"`
	ParentID         *snowflake.ID   `json:"parent_id,omitempty"`
}

func (ResolvedChannel) isMentionableValue() {}

type InteractionChannel struct {
	MessageChannel
	Permissions Permissions `json:"permissions"`
}

func (c *InteractionChannel) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	// One peek for the type and permissions, where each was probed by decoding the whole channel
	cType, rawPermissions := peek(data, "type", "permissions")

	var permissions Permissions
	if rawPermissions != nil {
		if err := permissions.UnmarshalJSON(rawPermissions); err != nil {
			return err
		}
	}

	channel, err := unmarshalChannel(data, ChannelType(peekInt(cType)))
	if err != nil {
		return err
	}
	msgChannel, ok := channel.(MessageChannel)
	if !ok {
		return fmt.Errorf("unknown channel type: %T", channel)
	}
	c.MessageChannel = msgChannel
	c.Permissions = permissions

	return nil
}

// UnmarshalJSONFrom decodes the channel as its type as it streams in, its type and permissions (a DM's has none)
// peeked in what dec has buffered. A channel not buffered that far is read whole and peeked there
func (c *InteractionChannel) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	cType, rawPermissions, ok := peekUnread(dec, "type", "permissions")
	if !ok {
		data, err := dec.ReadValue()
		if err != nil {
			return err
		}
		return c.UnmarshalJSON(data)
	}

	// Before the channel is read: the peeked values are dec's buffer
	var permissions Permissions
	if rawPermissions != nil {
		if err := permissions.UnmarshalJSON(rawPermissions); err != nil {
			return err
		}
	}

	channel, err := decodeChannel(ChannelType(peekInt(cType)), dec)
	if err != nil {
		return err
	}
	msgChannel, ok := channel.(MessageChannel)
	if !ok {
		return fmt.Errorf("unknown channel type: %T", channel)
	}
	c.MessageChannel = msgChannel
	c.Permissions = permissions

	return nil
}

func (c InteractionChannel) MarshalJSON() ([]byte, error) {
	// A ping has no channel, and the channel's null can't be merged with the permissions
	if c.MessageChannel == nil {
		return []byte("null"), nil
	}

	mData, err := json.Marshal(c.MessageChannel)
	if err != nil {
		return nil, err
	}

	pData, err := json.Marshal(struct {
		Permissions Permissions `json:"permissions"`
	}{
		Permissions: c.Permissions,
	})
	if err != nil {
		return nil, err
	}

	return json.Merge(mData, pData)
}

type InteractionGuild struct {
	ID       snowflake.ID   `json:"id"`
	Locale   Locale         `json:"locale"`
	Features []GuildFeature `json:"features"`
}
