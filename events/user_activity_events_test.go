package events

import (
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
)

func TestGenericUserActivityGuild(t *testing.T) {
	client := &bot.Client{Caches: cache.New(cache.WithCaches(cache.FlagGuilds))}
	client.Caches.AddGuild(discord.CacheGuild{Guild: discord.Guild{ID: 1}})

	event := &GenericUserActivity{
		GenericEvent: NewGenericEvent(client, 0, 0),
		UserID:       2,
		GuildID:      1,
	}

	guild, ok := event.Guild()
	if !ok || guild.ID != 1 {
		t.Errorf("Guild() = %d, %t; want 1, true", guild.ID, ok)
	}
}
