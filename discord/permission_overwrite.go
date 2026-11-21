package discord

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"strconv"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"
)

// PermissionOverwriteType is the type of PermissionOverwrite
type PermissionOverwriteType int

// Constants for PermissionOverwriteType
const (
	PermissionOverwriteTypeRole PermissionOverwriteType = iota
	PermissionOverwriteTypeMember
)

type PermissionOverwrites []PermissionOverwrite

func (p PermissionOverwrites) Get(overwriteType PermissionOverwriteType, id snowflake.ID) (PermissionOverwrite, bool) {
	for _, v := range p {
		if v.Type() == overwriteType && v.ID() == id {
			return v, true
		}
	}
	return nil, false
}

func (p PermissionOverwrites) Role(id snowflake.ID) (RolePermissionOverwrite, bool) {
	if overwrite, ok := p.Get(PermissionOverwriteTypeRole, id); ok {
		return overwrite.(RolePermissionOverwrite), true
	}
	return RolePermissionOverwrite{}, false
}

func (p PermissionOverwrites) Member(id snowflake.ID) (MemberPermissionOverwrite, bool) {
	if overwrite, ok := p.Get(PermissionOverwriteTypeMember, id); ok {
		return overwrite.(MemberPermissionOverwrite), true
	}
	return MemberPermissionOverwrite{}, false
}

// PermissionOverwrite is used to determine who can perform particular actions in a GetGuildChannel
type PermissionOverwrite interface {
	Type() PermissionOverwriteType
	ID() snowflake.ID
}

type UnmarshalPermissionOverwrite struct {
	PermissionOverwrite
}

func (o *UnmarshalPermissionOverwrite) UnmarshalJSON(data []byte) error {
	return unmarshalOnce(data, o)
}

// UnmarshalJSONFrom decodes the overwrite by hand, in one pass and without reflection: a server's channels carry
// hundreds, and through encoding/json/v2's reflection each cost more than its four fields are worth. Every kind has
// the fields UnknownPermissionOverwrite has; its type picks the kind after. As before, null is a role's overwrite
// with nothing set; names match byte for byte, where encoding/json folds case (Discord sends them lower case)
func (o *UnmarshalPermissionOverwrite) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	switch dec.PeekKind() {
	case 'n':
		o.PermissionOverwrite = RolePermissionOverwrite{}
		_, err := dec.ReadToken()
		return err
	case '{':
	default:
		// As the struct decoder words it, and reads past it
		return jsonv2.UnmarshalDecode(dec, new(RolePermissionOverwrite))
	}

	if _, err := dec.ReadToken(); err != nil {
		return err
	}

	var v UnknownPermissionOverwrite
	for dec.PeekKind() == '"' {
		name, err := dec.ReadValue()
		if err != nil {
			return err
		}

		// The name is dec's buffer, read over by the value: matched first. Each field is decoded by its own
		// method, called directly, so v stays on the stack
		var value jsontext.Value
		switch string(name) {
		case `"id"`:
			if value, err = dec.ReadValue(); err == nil {
				err = v.OverwriteID.UnmarshalJSON(value)
			}
		case `"type"`:
			if value, err = dec.ReadValue(); err == nil && value.Kind() != 'n' {
				var number int
				if number, err = strconv.Atoi(string(value)); err == nil {
					v.OverwriteType = PermissionOverwriteType(number)
				}
			}
		case `"allow"`:
			if value, err = dec.ReadValue(); err == nil {
				err = v.Allow.UnmarshalJSON(value)
			}
		case `"deny"`:
			if value, err = dec.ReadValue(); err == nil {
				err = v.Deny.UnmarshalJSON(value)
			}
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

	switch v.OverwriteType {
	case PermissionOverwriteTypeRole:
		o.PermissionOverwrite = RolePermissionOverwrite{RoleID: v.OverwriteID, Allow: v.Allow, Deny: v.Deny}
	case PermissionOverwriteTypeMember:
		o.PermissionOverwrite = MemberPermissionOverwrite{UserID: v.OverwriteID, Allow: v.Allow, Deny: v.Deny}
	default:
		o.PermissionOverwrite = v
	}
	return nil
}

type RolePermissionOverwrite struct {
	RoleID snowflake.ID `json:"id"`
	Allow  Permissions  `json:"allow"`
	Deny   Permissions  `json:"deny"`
}

func (o RolePermissionOverwrite) ID() snowflake.ID {
	return o.RoleID
}

func (o RolePermissionOverwrite) MarshalJSON() ([]byte, error) {
	type rolePermissionOverwrite RolePermissionOverwrite
	return json.Marshal(struct {
		Type PermissionOverwriteType `json:"type"`
		rolePermissionOverwrite
	}{
		Type:                    o.Type(),
		rolePermissionOverwrite: rolePermissionOverwrite(o),
	})
}

func (o RolePermissionOverwrite) Type() PermissionOverwriteType {
	return PermissionOverwriteTypeRole
}

type MemberPermissionOverwrite struct {
	UserID snowflake.ID `json:"id"`
	Allow  Permissions  `json:"allow"`
	Deny   Permissions  `json:"deny"`
}

func (o MemberPermissionOverwrite) ID() snowflake.ID {
	return o.UserID
}

func (o MemberPermissionOverwrite) MarshalJSON() ([]byte, error) {
	type memberPermissionOverwrite MemberPermissionOverwrite
	return json.Marshal(struct {
		Type PermissionOverwriteType `json:"type"`
		memberPermissionOverwrite
	}{
		Type:                      o.Type(),
		memberPermissionOverwrite: memberPermissionOverwrite(o),
	})
}

func (o MemberPermissionOverwrite) Type() PermissionOverwriteType {
	return PermissionOverwriteTypeMember
}

// UnknownPermissionOverwrite is a PermissionOverwrite with a type the library does not recognize yet.
type UnknownPermissionOverwrite struct {
	OverwriteType PermissionOverwriteType `json:"type"`
	OverwriteID   snowflake.ID            `json:"id"`
	Allow         Permissions             `json:"allow"`
	Deny          Permissions             `json:"deny"`
}

func (o UnknownPermissionOverwrite) ID() snowflake.ID {
	return o.OverwriteID
}

func (o UnknownPermissionOverwrite) Type() PermissionOverwriteType {
	return o.OverwriteType
}

type PermissionOverwriteUpdate interface {
	Type() PermissionOverwriteType
}

type RolePermissionOverwriteUpdate struct {
	Allow *Permissions `json:"allow,omitempty"`
	Deny  *Permissions `json:"deny,omitempty"`
}

func (u RolePermissionOverwriteUpdate) MarshalJSON() ([]byte, error) {
	type rolePermissionOverwriteUpdate RolePermissionOverwriteUpdate
	return json.Marshal(struct {
		Type PermissionOverwriteType `json:"type"`
		rolePermissionOverwriteUpdate
	}{
		Type:                          u.Type(),
		rolePermissionOverwriteUpdate: rolePermissionOverwriteUpdate(u),
	})
}

func (RolePermissionOverwriteUpdate) Type() PermissionOverwriteType {
	return PermissionOverwriteTypeRole
}

type MemberPermissionOverwriteUpdate struct {
	Allow *Permissions `json:"allow,omitempty"`
	Deny  *Permissions `json:"deny,omitempty"`
}

func (u MemberPermissionOverwriteUpdate) MarshalJSON() ([]byte, error) {
	type memberPermissionOverwriteUpdate MemberPermissionOverwriteUpdate
	return json.Marshal(struct {
		Type PermissionOverwriteType `json:"type"`
		memberPermissionOverwriteUpdate
	}{
		Type:                            u.Type(),
		memberPermissionOverwriteUpdate: memberPermissionOverwriteUpdate(u),
	})
}

func (MemberPermissionOverwriteUpdate) Type() PermissionOverwriteType {
	return PermissionOverwriteTypeMember
}
