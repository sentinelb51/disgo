package handlers

import (
	"fmt"
	"testing"

	"github.com/disgoorg/json/v2"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

// When the bot loses VIEW_CHANNEL on a channel, Discord sends no event for the channel's threads.
func TestChannelUpdateHidesThreadsOfForums(t *testing.T) {
	for _, channelType := range []discord.ChannelType{discord.ChannelTypeGuildText, discord.ChannelTypeGuildForum, discord.ChannelTypeGuildMedia} {
		client := newTestClient(t, cache.FlagGuilds|cache.FlagChannels|cache.FlagRoles|cache.FlagMembers)
		client.Caches.SetSelfUser(discord.OAuth2User{User: discord.User{ID: 5}})
		client.Caches.AddGuild(discord.CacheGuild{Guild: discord.Guild{ID: 1, OwnerID: 99}})
		client.Caches.AddRole(discord.Role{ID: 1, GuildID: 1, Permissions: discord.PermissionViewChannel})
		client.Caches.AddMember(discord.Member{GuildID: 1, User: discord.User{ID: 5}})
		var thread discord.GuildThread
		if err := json.Unmarshal([]byte(`{"id":"21","type":11,"guild_id":"1","parent_id":"20"}`), &thread); err != nil {
			t.Fatal(err)
		}
		client.Caches.AddChannel(thread)
		var hidden int
		client.AddEventListeners(bot.NewListenerFunc(func(*events.ThreadHide) { hidden++ }))

		// @everyone loses VIEW_CHANNEL (1024) on the parent
		var v discord.UnmarshalChannel
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"id":"20","type":%d,"guild_id":"1","permission_overwrites":[{"id":"1","type":0,"allow":"0","deny":"1024"}]}`, channelType)), &v); err != nil {
			t.Fatal(err)
		}
		gatewayHandlerChannelUpdate(client, 0, 0, gateway.EventChannelUpdate{GuildChannel: v.Channel.(discord.GuildChannel)})

		if _, ok := client.Caches.Channel(21); ok {
			t.Errorf("channel type %d: thread of a channel the bot can no longer view still cached", channelType)
		}
		if hidden != 1 {
			t.Errorf("channel type %d: ThreadHide dispatched %d times, want 1", channelType, hidden)
		}
	}
}
