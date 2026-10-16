package discord

import (
	"testing"
)

func TestGuildScheduledEvent_CoverURL(t *testing.T) {
	image := "5e1c1bb9a5d0f4bb"
	event := GuildScheduledEvent{ID: 1, Image: &image}

	want := "https://cdn.discordapp.com/guild-events/1/5e1c1bb9a5d0f4bb.png"
	got := event.CoverURL()
	if got == nil {
		t.Fatal("got no URL")
	}
	if *got != want {
		t.Errorf("got %s, want %s", *got, want)
	}
}
