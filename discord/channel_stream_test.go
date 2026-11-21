package discord

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"testing"
	"testing/iotest"
)

// Channels and an interaction's channel decode the same streamed as from bytes: the type peeked in the decoder's
// buffer, or, a byte at a time, where the buffer never holds it whole, read whole and peeked there
func TestChannelsStreamed(t *testing.T) {

	channels := []byte(`[
		{"type":0,"id":"1","guild_id":"9","name":"text","position":2,"permission_overwrites":[{"id":"9","type":0,"allow":"0","deny":"1024"},{"id":"5","type":1,"allow":"1024","deny":"0"}]},
		{"id":"2","guild_id":"9","name":"type last","type":2,"bitrate":64000},
		{"type":4,"id":"3","guild_id":"9","name":"category"},
		{"type":11,"id":"4","guild_id":"9","name":"thread","parent_id":"1","thread_metadata":{"archived":false,"auto_archive_duration":1440,"archive_timestamp":"2026-09-30T22:14:05+00:00","locked":false}},
		{"type":99,"id":"6","guild_id":"9","name":"unknown"}
	]`)

	interaction := []byte(`{"guild":{"type":0,"permissions":"2048","id":"1","name":"text"},"dm":{"type":1,"id":"2","recipients":[]},"late":{"id":"3","permissions":"8","name":"news","type":5}}`)

	type channels_ = []UnmarshalChannel
	type interactions = struct {
		Guild InteractionChannel `json:"guild"`
		DM    InteractionChannel `json:"dm"`
		Late  InteractionChannel `json:"late"`
	}

	for _, test := range []struct {
		name string
		data []byte
		into func() any
	}{
		{"channels", channels, func() any { return new(channels_) }},
		{"interaction channels", interaction, func() any { return new(interactions) }},
	} {
		want := test.into()
		if err := unmarshalOnce(test.data, want); err != nil {
			t.Fatalf("%s from bytes: %v", test.name, err)
		}

		for _, reader := range []struct {
			name string
			wrap func([]byte) *jsontext.Decoder
		}{
			{"buffered", func(data []byte) *jsontext.Decoder { return jsontext.NewDecoder(bytes.NewReader(data), decodeOptions) }},
			{"byte at a time", func(data []byte) *jsontext.Decoder {
				return jsontext.NewDecoder(iotest.OneByteReader(bytes.NewReader(data)), decodeOptions)
			}},
		} {
			got := test.into()
			if err := jsonv2.UnmarshalDecode(reader.wrap(test.data), got); err != nil {
				t.Fatalf("%s %s: %v", test.name, reader.name, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s:\n got %+v\nwant %+v", test.name, reader.name, got, want)
			}
		}
	}

	var decoded channels_
	if err := unmarshalOnce(channels, &decoded); err != nil {
		t.Fatal(err)
	}
	if voice, ok := decoded[1].Channel.(GuildVoiceChannel); !ok || voice.Bitrate() != 64000 {
		t.Errorf("type last: %#v", decoded[1].Channel)
	}
	if unknown, ok := decoded[4].Channel.(UnknownChannel); !ok || len(unknown.Data) == 0 {
		t.Errorf("unknown: %#v", decoded[4].Channel)
	}

	var channel interactions
	if err := unmarshalOnce(interaction, &channel); err != nil {
		t.Fatal(err)
	}
	if channel.Guild.Permissions != 2048 || channel.Late.Permissions != 8 || channel.DM.Permissions != 0 {
		t.Errorf("permissions: %d, %d, %d", channel.Guild.Permissions, channel.Late.Permissions, channel.DM.Permissions)
	}
	if _, ok := channel.DM.MessageChannel.(DMChannel); !ok {
		t.Errorf("dm: %#v", channel.DM.MessageChannel)
	}
}
