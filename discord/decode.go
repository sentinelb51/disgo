package discord

//go:generate go run ../internal/decodegen/cmd

import (
	"bytes"
	"encoding/binary"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"math/bits"
	"strconv"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/disgoorg/snowflake/v2"
)

/*
The runtime of the decoders decodegen writes (decode_gen.go): each reads an object token by token, a field's value
by the helper for its type, without reflection. Every helper takes the clear-cut case itself (a string without
escapes, a number that fits, a snowflake in quotes) and hands anything else to encoding/json/v2 with decodeOptions,
which so decides what any other input means and which errors it is: a generated decoder decodes as the reflection
decoder does, faster where Discord's payloads go.
*/

// decodeState is a generated decode's: the decoder it reads, and the short strings it decoded last, which a
// pooled state keeps from one decode to the next, as encoding/json/v2's decoder keeps its own
type decodeState struct {
	dec     *jsontext.Decoder
	strings [256]string
}

var decodeStates = sync.Pool{New: func() any { return new(decodeState) }}

// enterDecode is a state reading dec, for leaveDecode to return
func enterDecode(dec *jsontext.Decoder) *decodeState {
	d := decodeStates.Get().(*decodeState)
	d.dec = dec
	return d
}

func leaveDecode(d *decodeState) {
	d.dec = nil
	decodeStates.Put(d)
}

// intern is string(b), the cache's copy when it holds one, as encoding/json/v2's makeString: the usernames, hashes
// and types frames repeat cost no allocation
func (d *decodeState) intern(b []byte) string {
	const (
		minCachedLen = 2   // single bytes are the runtime's own
		maxCachedLen = 256 // UUIDs, hashes and URLs of a sort
	)
	if len(b) < minCachedLen || len(b) > maxCachedLen {
		return string(b)
	}

	var h uint32
	switch {
	case len(b) >= 8:
		lo, hi := binary.LittleEndian.Uint64(b[:8]), binary.LittleEndian.Uint64(b[len(b)-8:])
		h = hash64(uint32(lo), uint32(lo>>32)) ^ hash64(uint32(hi), uint32(hi>>32))
	case len(b) >= 4:
		h = hash64(binary.LittleEndian.Uint32(b[:4]), binary.LittleEndian.Uint32(b[len(b)-4:]))
	default:
		h = hash64(uint32(binary.LittleEndian.Uint16(b[:2])), uint32(binary.LittleEndian.Uint16(b[len(b)-2:])))
	}

	slot := &d.strings[h%uint32(len(d.strings))]
	if *slot == string(b) {
		return *slot
	}
	*slot = string(b)
	return *slot
}

// hash64 is XXH32's mix of eight bytes without its final avalanche, as encoding/json/v2's
func hash64(lo, hi uint32) uint32 {
	const (
		prime3 = 0xc2b2ae3d
		prime4 = 0x27d4eb2f
		prime5 = 0x165667b1
	)
	h := prime5 + uint32(8)
	h += lo * prime3
	h = bits.RotateLeft32(h, 17) * prime4
	h += hi * prime3
	return bits.RotateLeft32(h, 17) * prime4
}

// decodeFunc decodes the value d reads next into v
type decodeFunc[T any] func(d *decodeState, v *T) error

// delegate decodes value, already read, into v as encoding/json/v2 does
func delegate(value jsontext.Value, v any) error {
	return jsonv2.Unmarshal(value, v, decodeOptions)
}

// beginObject reads the '{' of the object d reads next, and reports whether there are members to read. null leaves
// v as it is, as encoding/json does; anything else is decoded into plain (v, which has no methods) by
// encoding/json/v2, which words its error
func beginObject(d *decodeState, plain any) (bool, error) {
	switch d.dec.PeekKind() {
	case '{':
		_, err := d.dec.ReadToken()
		return err == nil, err
	case 'n':
		_, err := d.dec.ReadToken()
		return false, err
	}

	value, err := d.dec.ReadValue()
	if err != nil {
		return false, err
	}
	return false, delegate(value, plain)
}

// endObject reads the '}' after the last member
func endObject(d *decodeState) error {
	_, err := d.dec.ReadToken()
	return err
}

// otherMember reads the value of a member no field takes by its exact name. A plain lower-case name is no field's,
// and is skipped; one encoding/json might still match (by case, or escaped) is decoded into plain as a one-member
// object by encoding/json/v2
func otherMember(d *decodeState, name jsontext.Value, plain any) error {
	if plainName(name) {
		return d.dec.SkipValue()
	}

	// name is the decoder's buffer, which the value's read reuses: otherKey copies it first
	return otherKey(d, name, plain)
}

// plainName reports whether a quoted member name is lower-case ASCII letters, digits and underscores alone: such a
// name matches a field only exactly, which the generated switch tried
func plainName(name jsontext.Value) bool {
	for _, c := range name[1 : len(name)-1] {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// otherKey hands a member, its value unread, to encoding/json/v2 as a one-member object decoded into v
func otherKey(d *decodeState, key jsontext.Value, v any) error {
	object := append([]byte{'{'}, key...)
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}
	object = append(append(append(object, ':'), value...), '}')
	return delegate(object, v)
}

// quoted is a string value's bytes when they need no unquoting (no escapes, valid UTF-8), and false otherwise
func quoted(value jsontext.Value) ([]byte, bool) {
	if len(value) < 2 || value[0] != '"' {
		return nil, false
	}
	inner := value[1 : len(value)-1]
	if bytes.IndexByte(inner, '\\') >= 0 || !utf8.Valid(inner) {
		return nil, false
	}
	return inner, true
}

func decodeString[T ~string](d *decodeState, v *T) error {
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}

	switch value.Kind() {
	case 'n':
		return nil
	case '"':
		if inner, ok := quoted(value); ok {
			*v = T(d.intern(inner))
			return nil
		}
		// The unquoted bytes are the string's own, never written again: no second copy
		if unquoted, err := jsontext.AppendUnquote(nil, value); err == nil {
			*v = T(unsafe.String(unsafe.SliceData(unquoted), len(unquoted)))
			return nil
		}
	}
	return delegate(value, v)
}

func decodeBool[T ~bool](d *decodeState, v *T) error {
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}

	switch value.Kind() {
	case 'n':
		return nil
	case 't':
		*v = true
		return nil
	case 'f':
		*v = false
		return nil
	}
	return delegate(value, v)
}

func decodeInt[T ~int | ~int8 | ~int16 | ~int32 | ~int64](d *decodeState, v *T) error {
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}

	switch value.Kind() {
	case 'n':
		return nil
	case '0':
		if number, err := strconv.ParseInt(string(value), 10, int(unsafe.Sizeof(*v))*8); err == nil {
			*v = T(number)
			return nil
		}
	}
	return delegate(value, v)
}

func decodeFloat[T ~float32 | ~float64](d *decodeState, v *T) error {
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}

	switch value.Kind() {
	case 'n':
		return nil
	case '0':
		if number, err := strconv.ParseFloat(string(value), int(unsafe.Sizeof(*v))*8); err == nil {
			*v = T(number)
			return nil
		}
	}
	return delegate(value, v)
}

// decodeID decodes a snowflake: quoted digits are parsed in place, anything else goes to its UnmarshalJSON
func decodeID(d *decodeState, v *snowflake.ID) error {
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}

	if id, ok := parseID(value); ok {
		*v = id
		return nil
	}
	return v.UnmarshalJSON(value)
}

// parseID parses a quoted snowflake of up to 19 digits, which always fits
func parseID(value jsontext.Value) (snowflake.ID, bool) {
	digits, ok := quoted(value)
	if !ok || len(digits) == 0 || len(digits) > 19 {
		return 0, false
	}

	var id uint64
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		id = id*10 + uint64(c-'0')
	}
	return snowflake.ID(id), true
}

// jsonMethod decodes a value whose pointer has an UnmarshalJSON method, handed the value as encoding/json/v2
// hands it, null included
func jsonMethod[T any, P interface {
	*T
	UnmarshalJSON([]byte) error
}](d *decodeState, v *T) error {
	value, err := d.dec.ReadValue()
	if err != nil {
		return err
	}
	return P(v).UnmarshalJSON(value)
}

// fromMethod decodes a value whose pointer has an UnmarshalJSONFrom method, as encoding/json/v2 calls it
func fromMethod[T any, P interface {
	*T
	UnmarshalJSONFrom(*jsontext.Decoder) error
}](d *decodeState, v *T) error {
	return P(v).UnmarshalJSONFrom(d.dec)
}

// reflected decodes a value by encoding/json/v2's reflection: a type the generator doesn't decode itself
func reflected[T any](d *decodeState, v *T) error {
	return jsonv2.UnmarshalDecode(d.dec, v)
}

// decodePtr decodes into the value a pointer points at, made when nil, as encoding/json does; null sets it nil
func decodePtr[T any](d *decodeState, v **T, decode decodeFunc[T]) error {
	if d.dec.PeekKind() == 'n' {
		_, err := d.dec.ReadToken()
		*v = nil
		return err
	}

	if *v == nil {
		*v = new(T)
	}
	return decode(d, *v)
}

// decodeSlice decodes an array as encoding/json does: into the elements of the slice's capacity, as they are (a
// member repeated merges into the last one's), new zero ones after, and cut to the array's length; an empty array
// is an empty slice of no capacity, null nil
func decodeSlice[T any](d *decodeState, v *[]T, decode decodeFunc[T]) error {
	switch d.dec.PeekKind() {
	case 'n':
		_, err := d.dec.ReadToken()
		*v = nil
		return err
	case '[':
	default:
		value, err := d.dec.ReadValue()
		if err != nil {
			return err
		}
		return delegate(value, v)
	}

	if _, err := d.dec.ReadToken(); err != nil {
		return err
	}

	slice := (*v)[:cap(*v)]
	count := 0
	for d.dec.PeekKind() != ']' {
		if count == len(slice) {
			// append zeroes the capacity it adds
			var zero T
			slice = append(slice, zero)
			slice = slice[:cap(slice)]
		}
		if err := decode(d, &slice[count]); err != nil {
			*v = slice[:count]
			return err
		}
		count++
	}

	if _, err := d.dec.ReadToken(); err != nil {
		return err
	}

	if count == 0 {
		*v = []T{}
		return nil
	}
	*v = slice[:count]
	return nil
}

// decodeMapID decodes an object into a map keyed by snowflakes as encoding/json does: made when nil, each value
// decoded into a zero one and set, a key of quoted digits parsed in place; any other key goes with its value to
// encoding/json/v2, merged into the map. null sets it nil
func decodeMapID[V any](d *decodeState, v *map[snowflake.ID]V, decode decodeFunc[V]) error {
	switch d.dec.PeekKind() {
	case 'n':
		_, err := d.dec.ReadToken()
		*v = nil
		return err
	case '{':
	default:
		value, err := d.dec.ReadValue()
		if err != nil {
			return err
		}
		return delegate(value, v)
	}

	if _, err := d.dec.ReadToken(); err != nil {
		return err
	}
	if *v == nil {
		*v = make(map[snowflake.ID]V)
	}

	for d.dec.PeekKind() == '"' {
		key, err := d.dec.ReadValue()
		if err != nil {
			return err
		}

		id, ok := parseID(key)
		if !ok {
			if err = otherKey(d, key, v); err != nil {
				return err
			}
			continue
		}

		var value V
		if err = decode(d, &value); err != nil {
			return err
		}
		(*v)[id] = value
	}

	_, err := d.dec.ReadToken()
	return err
}
