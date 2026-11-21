package discord

import (
	"encoding/json"
	"testing"
)

// A channel of a type this package doesn't model (14, GUILD_DIRECTORY, in a hub) must not cost the whole guild
func TestGatewayGuild_UnmarshalJSONUnknownChannelType(t *testing.T) {
	data := []byte(`{"id":"1","name":"hub","channels":[
		{"id":"2","type":0,"guild_id":"1","name":"general","position":0,"permission_overwrites":[]},
		{"id":"3","type":14,"guild_id":"1","name":"directory","position":1,"permission_overwrites":[]}
	]}`)

	var guild GatewayGuild
	if err := json.Unmarshal(data, &guild); err != nil {
		t.Fatalf("the guild failed to decode: %v", err)
	}

	if len(guild.Channels) != 2 || guild.Channels[0].ID() != 2 {
		t.Fatalf("got channels %+v, want the text channel and the directory", guild.Channels)
	}
	if _, ok := guild.Channels[1].(UnknownChannel); !ok {
		t.Fatalf("got %T, want the directory as an UnknownChannel", guild.Channels[1])
	}
}
