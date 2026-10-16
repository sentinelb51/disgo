package handlers

import (
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
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

func TestThreadMemberUpdate(t *testing.T) {
	// no thread cached: the guild comes from the payload
	client := newTestClient(t, cache.FlagThreadMembers)
	client.Caches.AddThreadMember(discord.ThreadMember{ThreadID: 100, UserID: 5, Flags: 1})
	var got *events.ThreadMemberUpdate
	client.AddEventListeners(bot.NewListenerFunc(func(e *events.ThreadMemberUpdate) { got = e }))

	GetGatewayHandlers()[gateway.EventTypeThreadMemberUpdate].HandleGatewayEvent(client, 0, 0, gateway.EventThreadMemberUpdate{
		ThreadMember: discord.ThreadMember{ThreadID: 100, UserID: 5, Flags: 2},
		GuildID:      1,
	})

	if member, ok := client.Caches.ThreadMember(100, 5); !ok || member.Flags != 2 {
		t.Errorf("cached thread member = %+v, %t, want flags 2", member, ok)
	}
	if got == nil {
		t.Fatal("ThreadMemberUpdate not dispatched")
	}
	if got.GuildID != 1 || got.ThreadID != 100 || got.ThreadMemberID != 5 || got.ThreadMember.Flags != 2 || got.OldThreadMember.Flags != 1 {
		t.Errorf("ThreadMemberUpdate = %+v, old %+v", got.GenericThreadMember, got.OldThreadMember)
	}
}
