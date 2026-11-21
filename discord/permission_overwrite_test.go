package discord

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"testing"
	"testing/iotest"
)

// Overwrites, decoded by hand, decode as the struct decoder decoded each kind: every field, in any order, with
// unknown ones skipped and null as nothing set
func TestPermissionOverwritesDecoded(t *testing.T) {

	data := []byte(`[
		{"id":"1","type":0,"allow":"1024","deny":"2048"},
		{"deny":"8","allow":"0","id":"2","type":1,"extra":{"nested":[1,"two",null]}},
		{"type":7,"id":"3","allow":"16","deny":"0"},
		{"id":"4","type":null,"allow":null,"deny":"32"},
		{"id":"5"},
		null
	]`)

	want := []UnmarshalPermissionOverwrite{
		{RolePermissionOverwrite{RoleID: 1, Allow: 1024, Deny: 2048}},
		{MemberPermissionOverwrite{UserID: 2, Allow: 0, Deny: 8}},
		{UnknownPermissionOverwrite{OverwriteType: 7, OverwriteID: 3, Allow: 16}},
		{RolePermissionOverwrite{RoleID: 4, Deny: 32}},
		{RolePermissionOverwrite{RoleID: 5}},
		{RolePermissionOverwrite{}},
	}

	var fromBytes []UnmarshalPermissionOverwrite
	if err := unmarshalOnce(data, &fromBytes); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromBytes, want) {
		t.Errorf("from bytes:\n got %+v\nwant %+v", fromBytes, want)
	}

	var streamed []UnmarshalPermissionOverwrite
	if err := jsonv2.UnmarshalDecode(jsontext.NewDecoder(iotest.OneByteReader(bytes.NewReader(data)), decodeOptions), &streamed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(streamed, want) {
		t.Errorf("streamed:\n got %+v\nwant %+v", streamed, want)
	}

	for _, bad := range []string{`{"id":"1","type":"0"}`, `{"id":1}`, `{"allow":true}`, `5`} {
		var overwrite UnmarshalPermissionOverwrite
		if err := unmarshalOnce([]byte(bad), &overwrite); err == nil {
			t.Errorf("%s: decoded as %+v", bad, overwrite)
		}
	}
}
