package discord

import (
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestUser_DefaultAvatarURL(t *testing.T) {
	id := snowflake.ID(1 << 22) // (id >> 22) % 6 == 1

	tests := []struct {
		discriminator string
		want          string
	}{
		{"0", "https://cdn.discordapp.com/embed/avatars/1.png"},    // new username system: (id >> 22) % 6
		{"0001", "https://cdn.discordapp.com/embed/avatars/1.png"}, // legacy: discriminator % 5
		{"1235", "https://cdn.discordapp.com/embed/avatars/0.png"},
		{"0005", "https://cdn.discordapp.com/embed/avatars/0.png"},
		{"9999", "https://cdn.discordapp.com/embed/avatars/4.png"},
	}
	for _, tt := range tests {
		t.Run(tt.discriminator, func(t *testing.T) {
			user := User{ID: id, Discriminator: tt.discriminator}
			if got := user.DefaultAvatarURL(); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
