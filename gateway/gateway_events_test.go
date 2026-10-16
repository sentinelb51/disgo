package gateway

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/disgoorg/json/v2"
)

// THREAD_MEMBER_UPDATE is a thread member with the guild_id of its thread
func TestEventThreadMemberUpdateGuildID(t *testing.T) {
	frame := `{"op":0,"s":5,"t":"THREAD_MEMBER_UPDATE","d":{"id":"1300000000000000001","user_id":"1300000000000000002","join_timestamp":"2026-10-15T12:00:00.000000+00:00","flags":1,"guild_id":"1300000000000000003"}}`

	var message Message
	if err := jsonv2.UnmarshalDecode(jsontext.NewDecoder(bytes.NewReader([]byte(frame)), decodeOptions), &message); err != nil {
		t.Fatal(err)
	}
	event, ok := message.D.(EventThreadMemberUpdate)
	if !ok {
		t.Fatalf("data: %T", message.D)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if event.GuildID != 1300000000000000003 || !bytes.Contains(data, []byte(`"guild_id":"1300000000000000003"`)) {
		t.Fatalf("guild_id lost: %s", data)
	}
	if !bytes.Contains(data, []byte(`"id":"1300000000000000001"`)) || !bytes.Contains(data, []byte(`"user_id":"1300000000000000002"`)) {
		t.Fatalf("thread member lost: %s", data)
	}
}
