package handlers

import (
	"log/slog"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
)

// newTestClient returns a client with real caches and event manager, enough to run the gateway handlers directly.
func newTestClient(t *testing.T, flags cache.Flags) *bot.Client {
	t.Helper()
	client := &bot.Client{
		Logger: slog.New(slog.DiscardHandler),
		Caches: cache.New(cache.WithCaches(flags)),
	}
	client.EventManager = bot.NewEventManager(client)
	client.MemberChunkingManager = bot.NewMemberChunkingManager(client, client.Logger, nil)
	return client
}
