package handlers

import (
	"fmt"
	"testing"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

// A guild listed in READY that has an outage before its GUILD_CREATE gets a GUILD_DELETE with unavailable set, then a
// GUILD_CREATE once it is back.
func TestGuildCreateAfterOutageWhileUnready(t *testing.T) {
	client := newTestClient(t, cache.FlagGuilds)
	var ready int
	client.AddEventListeners(bot.NewListenerFunc(func(*events.GuildReady) { ready++ }))

	gatewayHandlerReady(client, 0, 0, gateway.EventReady{Guilds: []discord.UnavailableGuild{{ID: 1, Unavailable: true}}})
	gatewayHandlerGuildDelete(client, 0, 0, gateway.EventGuildDelete{UnavailableGuild: discord.UnavailableGuild{ID: 1, Unavailable: true}})
	gatewayHandlerGuildCreate(client, 0, 0, gateway.EventGuildCreate{GatewayGuild: discord.GatewayGuild{
		CacheGuild: discord.CacheGuild{Guild: discord.Guild{ID: 1, Name: "back"}},
	}})

	if client.Caches.IsGuildUnavailable(1) {
		t.Error("guild still marked unavailable after its GUILD_CREATE")
	}
	if client.Caches.IsGuildUnready(1) {
		t.Error("guild still marked unready after its GUILD_CREATE")
	}
	if ready != 1 {
		t.Errorf("GuildReady dispatched %d times, want 1", ready)
	}
}

// During an outage, a GUILD_CREATE sent on connect or on join may carry only an unavailable guild object.
func TestGuildCreateUnavailable(t *testing.T) {
	client := newTestClient(t, cache.FlagGuilds)
	var ready, joined int
	client.AddEventListeners(
		bot.NewListenerFunc(func(*events.GuildReady) { ready++ }),
		bot.NewListenerFunc(func(*events.GuildJoin) { joined++ }),
	)

	gatewayHandlerReady(client, 0, 0, gateway.EventReady{Guilds: []discord.UnavailableGuild{{ID: 1, Unavailable: true}}})
	gatewayHandlerGuildCreate(client, 0, 0, gateway.EventGuildCreate{GatewayGuild: discord.GatewayGuild{
		CacheGuild:  discord.CacheGuild{Guild: discord.Guild{ID: 1}},
		Unavailable: true,
	}})

	if n := client.Caches.GuildsLen(); n != 0 {
		t.Errorf("GuildsLen = %d after an unavailable GUILD_CREATE, want 0", n)
	}
	if !client.Caches.IsGuildUnavailable(1) {
		t.Error("guild not marked unavailable")
	}
	if ready != 0 || joined != 0 {
		t.Errorf("GuildReady/GuildJoin dispatched %d/%d times for an unavailable guild, want 0/0", ready, joined)
	}

	gatewayHandlerGuildCreate(client, 0, 0, gateway.EventGuildCreate{GatewayGuild: discord.GatewayGuild{
		CacheGuild: discord.CacheGuild{Guild: discord.Guild{ID: 1, Name: "back"}},
	}})

	if guild, ok := client.Caches.Guild(1); !ok || guild.Name != "back" {
		t.Errorf("guild = %+v, %t after it came back", guild.Guild, ok)
	}
	if client.Caches.IsGuildUnavailable(1) || client.Caches.IsGuildUnready(1) {
		t.Error("guild still marked unavailable or unready after it came back")
	}
	if ready != 1 || joined != 0 {
		t.Errorf("GuildReady/GuildJoin dispatched %d/%d times, want 1/0", ready, joined)
	}
}

func guildCreate(t *testing.T, guildID snowflake.ID, channelIDs ...snowflake.ID) gateway.EventGuildCreate {
	t.Helper()
	guild := discord.GatewayGuild{CacheGuild: discord.CacheGuild{Guild: discord.Guild{ID: guildID}}}
	for _, channelID := range channelIDs {
		var v discord.UnmarshalChannel
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"id":"%d","type":0}`, channelID)), &v); err != nil {
			t.Fatal(err)
		}
		guild.Channels = append(guild.Channels, v.Channel.(discord.GuildChannel))
	}
	guild.Roles = []discord.Role{{ID: guildID}}
	return gateway.EventGuildCreate{GatewayGuild: guild}
}

// After a new session (READY without RESUME), Discord sends a GUILD_CREATE for each guild the bot is still in, and
// nothing for what changed while it was disconnected.
func TestReadyDropsPreviousSession(t *testing.T) {
	client := newTestClient(t, cache.FlagGuilds|cache.FlagChannels|cache.FlagRoles)
	var guildsReady int
	client.AddEventListeners(bot.NewListenerFunc(func(*events.GuildsReady) { guildsReady++ }))

	gatewayHandlerReady(client, 0, 0, gateway.EventReady{Shard: [2]int{0, 1}, Guilds: []discord.UnavailableGuild{{ID: 1}, {ID: 2}, {ID: 3}}})
	gatewayHandlerGuildCreate(client, 0, 0, guildCreate(t, 1, 10))
	gatewayHandlerGuildCreate(client, 0, 0, guildCreate(t, 2, 20))
	// the session dies before guild 3 arrives; while disconnected the bot leaves guilds 2 and 3, and channel 10 is
	// replaced by 11
	gatewayHandlerReady(client, 0, 0, gateway.EventReady{Shard: [2]int{0, 1}, Guilds: []discord.UnavailableGuild{{ID: 1}}})
	gatewayHandlerGuildCreate(client, 0, 0, guildCreate(t, 1, 11))

	if n := client.Caches.GuildsLen(); n != 1 {
		t.Errorf("GuildsLen = %d, want 1", n)
	}
	if _, ok := client.Caches.Guild(2); ok {
		t.Error("guild left while disconnected still cached")
	}
	if _, ok := client.Caches.Channel(20); ok {
		t.Error("channel of a guild left while disconnected still cached")
	}
	if _, ok := client.Caches.Channel(10); ok {
		t.Error("channel deleted while disconnected still cached")
	}
	if _, ok := client.Caches.Channel(11); !ok {
		t.Error("channel from the new GUILD_CREATE not cached")
	}
	if n := client.Caches.RolesLen(2); n != 0 {
		t.Errorf("%d roles of a guild left while disconnected still cached", n)
	}
	if client.Caches.IsGuildUnready(3) {
		t.Error("guild left before its GUILD_CREATE still unready")
	}
	if guildsReady != 1 {
		t.Errorf("GuildsReady dispatched %d times, want 1 (the second session)", guildsReady)
	}
}

// With several shards sharing the caches, a shard's READY only drops the guilds of that shard.
func TestReadyKeepsOtherShards(t *testing.T) {
	client := newTestClient(t, cache.FlagGuilds|cache.FlagChannels)
	shard0Guild, shard1Guild := snowflake.ID(2<<22), snowflake.ID(1<<22)

	gatewayHandlerReady(client, 0, 1, gateway.EventReady{Shard: [2]int{1, 2}, Guilds: []discord.UnavailableGuild{{ID: shard1Guild}}})
	gatewayHandlerGuildCreate(client, 0, 1, guildCreate(t, shard1Guild, 10))
	gatewayHandlerReady(client, 0, 0, gateway.EventReady{Shard: [2]int{0, 2}, Guilds: []discord.UnavailableGuild{{ID: shard0Guild}}})

	if _, ok := client.Caches.Guild(shard1Guild); !ok {
		t.Error("shard 0's READY dropped a guild of shard 1")
	}
	if _, ok := client.Caches.Channel(10); !ok {
		t.Error("shard 0's READY dropped a channel of shard 1")
	}
}

func TestGuildDeleteRemovesGuildEntities(t *testing.T) {
	client := newTestClient(t, cache.FlagGuilds|cache.FlagChannels|cache.FlagRoles|cache.FlagThreadMembers|cache.FlagMessages|cache.FlagEmojis)
	gatewayHandlerGuildCreate(client, 0, 0, guildCreate(t, 1, 10))
	gatewayHandlerGuildCreate(client, 0, 0, guildCreate(t, 2, 20))
	var thread discord.GuildThread
	if err := json.Unmarshal([]byte(`{"id":"11","type":11,"guild_id":"1","parent_id":"10"}`), &thread); err != nil {
		t.Fatal(err)
	}
	client.Caches.AddChannel(thread)
	client.Caches.AddThreadMember(discord.ThreadMember{ThreadID: 11, UserID: 5})
	guildID := snowflake.ID(1)
	client.Caches.AddMessage(discord.Message{ID: 100, ChannelID: 10, GuildID: &guildID})
	client.Caches.AddEmoji(discord.Emoji{ID: 7, GuildID: 1})

	gatewayHandlerGuildDelete(client, 0, 0, gateway.EventGuildDelete{UnavailableGuild: discord.UnavailableGuild{ID: 1}})

	if _, ok := client.Caches.Guild(1); ok {
		t.Error("guild still cached")
	}
	for _, channelID := range []snowflake.ID{10, 11} {
		if _, ok := client.Caches.Channel(channelID); ok {
			t.Errorf("channel %d still cached", channelID)
		}
	}
	if n := client.Caches.RolesLen(1) + client.Caches.EmojisLen(1) + client.Caches.ThreadMembersLen(11) + client.Caches.MessagesAllLen(); n != 0 {
		t.Errorf("%d roles, emojis, thread members or messages of the guild still cached", n)
	}
	if _, ok := client.Caches.Channel(20); !ok {
		t.Error("channel of another guild removed")
	}
	if n := client.Caches.RolesLen(2); n != 1 {
		t.Errorf("RolesLen(other guild) = %d, want 1", n)
	}
}

// Leaving a guild drops it from the unready and unavailable sets, so GuildsReady still fires when it was the last
// unready guild.
func TestGuildDeleteLeftForgetsState(t *testing.T) {
	client := newTestClient(t, cache.FlagGuilds)
	var guildsReady int
	client.AddEventListeners(bot.NewListenerFunc(func(*events.GuildsReady) { guildsReady++ }))

	gatewayHandlerReady(client, 0, 0, gateway.EventReady{Shard: [2]int{0, 1}, Guilds: []discord.UnavailableGuild{{ID: 1}, {ID: 2}, {ID: 3}}})
	gatewayHandlerGuildCreate(client, 0, 0, guildCreate(t, 1))
	gatewayHandlerGuildDelete(client, 0, 0, gateway.EventGuildDelete{UnavailableGuild: discord.UnavailableGuild{ID: 3, Unavailable: true}})
	gatewayHandlerGuildDelete(client, 0, 0, gateway.EventGuildDelete{UnavailableGuild: discord.UnavailableGuild{ID: 2}})
	gatewayHandlerGuildDelete(client, 0, 0, gateway.EventGuildDelete{UnavailableGuild: discord.UnavailableGuild{ID: 3}})

	for _, guildID := range []snowflake.ID{2, 3} {
		if client.Caches.IsGuildUnready(guildID) || client.Caches.IsGuildUnavailable(guildID) {
			t.Errorf("guild %d left but still unready or unavailable", guildID)
		}
	}
	if guildsReady != 1 {
		t.Errorf("GuildsReady dispatched %d times, want 1", guildsReady)
	}
}
