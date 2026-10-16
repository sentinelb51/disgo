package handlers

import (
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

// A guild that loses boosts gets a GUILD_EMOJIS_UPDATE marking the emojis over its new limit unavailable.
func TestGuildEmojisUpdateAvailability(t *testing.T) {
	client := newTestClient(t, cache.FlagEmojis)
	client.Caches.AddEmoji(discord.Emoji{ID: 10, GuildID: 1, Name: "e", Available: true})
	var updates int
	client.AddEventListeners(bot.NewListenerFunc(func(*events.EmojiUpdate) { updates++ }))

	gatewayHandlerGuildEmojisUpdate(client, 0, 0, gateway.EventGuildEmojisUpdate{
		GuildID: 1,
		Emojis:  []discord.Emoji{{ID: 10, GuildID: 1, Name: "e", Available: false}},
	})

	if emoji, ok := client.Caches.Emoji(1, 10); !ok || emoji.Available {
		t.Errorf("cached emoji = %+v, %t, want it unavailable", emoji, ok)
	}
	if updates != 1 {
		t.Errorf("EmojiUpdate dispatched %d times, want 1", updates)
	}
}
