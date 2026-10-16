package handlers

import (
	"testing"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

// The member in a voice state carries no guild_id; only the voice state does.
func TestVoiceStateUpdateCachesMemberInGuild(t *testing.T) {
	client := newTestClient(t, cache.FlagMembers|cache.FlagVoiceStates)
	channelID := snowflake.ID(2)

	gatewayHandlerVoiceStateUpdate(client, 0, 0, gateway.EventVoiceStateUpdate{
		VoiceState: discord.VoiceState{GuildID: 1, ChannelID: &channelID, UserID: 5},
		Member:     discord.Member{User: discord.User{ID: 5}},
	})

	if member, ok := client.Caches.Member(1, 5); !ok || member.GuildID != 1 {
		t.Errorf("Member(guild, user) = %+v, %t, want the member with GuildID 1", member, ok)
	}
	if n := client.Caches.MembersLen(0); n != 0 {
		t.Errorf("%d members cached under guild 0", n)
	}
}
