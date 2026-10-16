package handlers

import (
	"testing"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

func TestGuildStickersUpdateUpdatesCache(t *testing.T) {
	client := newTestClient(t, cache.FlagStickers)
	guildID := snowflake.ID(1)

	client.Caches.AddSticker(discord.Sticker{ID: 10, GuildID: &guildID, Name: "kept"})
	client.Caches.AddSticker(discord.Sticker{ID: 11, GuildID: &guildID, Name: "renamed"})
	client.Caches.AddSticker(discord.Sticker{ID: 12, GuildID: &guildID, Name: "deleted"})

	gatewayHandlerGuildStickersUpdate(client, 0, 0, gateway.EventGuildStickersUpdate{
		GuildID: guildID,
		Stickers: []discord.Sticker{
			{ID: 10, GuildID: &guildID, Name: "kept"},
			{ID: 11, GuildID: &guildID, Name: "new name"},
			{ID: 13, GuildID: &guildID, Name: "created"},
		},
	})

	if n := client.Caches.StickersLen(guildID); n != 3 {
		t.Errorf("StickersLen = %d, want 3", n)
	}
	if sticker, ok := client.Caches.Sticker(guildID, 11); !ok || sticker.Name != "new name" {
		t.Errorf("updated sticker = %+v, %t, want name %q", sticker, ok, "new name")
	}
	if _, ok := client.Caches.Sticker(guildID, 12); ok {
		t.Error("deleted sticker still cached")
	}
	if _, ok := client.Caches.Sticker(guildID, 13); !ok {
		t.Error("created sticker not cached")
	}
}
