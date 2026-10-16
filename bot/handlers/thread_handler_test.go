package handlers

import (
	"testing"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

func TestThreadMembersUpdateCachesMemberInGuild(t *testing.T) {
	client := newTestClient(t, cache.FlagMembers|cache.FlagThreadMembers)

	gatewayHandlerThreadMembersUpdate(client, 0, 0, gateway.EventThreadMembersUpdate{
		ID:      100,
		GuildID: 1,
		AddedMembers: []gateway.AddedThreadMember{{
			ThreadMember: discord.ThreadMember{ThreadID: 100, UserID: 5},
			Member:       discord.Member{User: discord.User{ID: 5}},
		}},
	})

	if member, ok := client.Caches.Member(1, 5); !ok || member.GuildID != 1 {
		t.Errorf("Member(guild, user) = %+v, %t, want the member with GuildID 1", member, ok)
	}
	if _, ok := client.Caches.Member(100, 5); ok {
		t.Error("member cached under the thread ID")
	}
}
