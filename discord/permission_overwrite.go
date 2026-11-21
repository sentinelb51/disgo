package discord

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"

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
	// The type is peeked rather than probed, and the overwrite decoded in one pass, as channels are
	oType, _ := peek(data, "type", "")
	overwrite, err := decodeOverwrite(PermissionOverwriteType(peekInt(oType)), func(v any) error { return unmarshalOnce(data, v) })
	if err != nil {
		return err
	}

	o.PermissionOverwrite = overwrite
	return nil
}

// UnmarshalJSONFrom decodes the overwrite as its type as it streams in, the type peeked in what dec has buffered
func (o *UnmarshalPermissionOverwrite) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	oType, _, ok := peekUnread(dec, "type", "")
	if !ok {
		data, err := dec.ReadValue()
		if err != nil {
			return err
		}
		return o.UnmarshalJSON(data)
	}

	overwrite, err := decodeOverwrite(PermissionOverwriteType(peekInt(oType)), func(v any) error { return jsonv2.UnmarshalDecode(dec, v) })
	if err != nil {
		return err
	}

	o.PermissionOverwrite = overwrite
	return nil
}

// decodeOverwrite decodes an overwrite of oType through decode, which reads one JSON value into the pointer it is
// handed
func decodeOverwrite(oType PermissionOverwriteType, decode func(v any) error) (PermissionOverwrite, error) {
	var (
		overwrite PermissionOverwrite
		err       error
	)

	switch oType {
	case PermissionOverwriteTypeRole:
		var v RolePermissionOverwrite
		err = decode(&v)
		overwrite = v

	case PermissionOverwriteTypeMember:
		var v MemberPermissionOverwrite
		err = decode(&v)
		overwrite = v

	default:
		var v UnknownPermissionOverwrite
		err = decode(&v)
		overwrite = v
	}

	if err != nil {
		return nil, err
	}
	return overwrite, nil
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
