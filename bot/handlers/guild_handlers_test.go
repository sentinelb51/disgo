package handlers

import (
	"testing"

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
