package handlers

import (
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

func TestPresenceUpdateActivityUpdate(t *testing.T) {
	client := newTestClient(t, cache.FlagPresences)
	var updates int
	client.AddEventListeners(bot.NewListenerFunc(func(*events.UserActivityUpdate) { updates++ }))

	activity := discord.Activity{
		ID:         "a",
		Name:       "game",
		CreatedAt:  time.Unix(1700000000, 0),
		Timestamps: &discord.ActivityTimestamps{Start: time.Unix(1700000000, 0)},
		Buttons:    []string{"join"},
	}
	presence := func(activity discord.Activity) gateway.EventPresenceUpdate {
		return gateway.EventPresenceUpdate{Presence: discord.Presence{
			PresenceUser: discord.PresenceUser{ID: 5},
			GuildID:      1,
			Status:       discord.OnlineStatusOnline,
			Activities:   []discord.Activity{activity},
		}}
	}

	gatewayHandlerPresenceUpdate(client, 0, 0, presence(activity))
	gatewayHandlerPresenceUpdate(client, 0, 0, presence(activity))
	if updates != 0 {
		t.Errorf("UserActivityUpdate dispatched %d times for an unchanged activity, want 0", updates)
	}

	changed := activity
	changed.CreatedAt = activity.CreatedAt.Add(time.Second)
	gatewayHandlerPresenceUpdate(client, 0, 0, presence(changed))
	if updates != 1 {
		t.Errorf("UserActivityUpdate dispatched %d times after created_at changed, want 1", updates)
	}
}
