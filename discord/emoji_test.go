package discord

import (
	"testing"
)

func TestEmoji_Reaction(t *testing.T) {
	tests := []struct {
		name         string
		emoji        Emoji
		wantReaction string
		wantMention  string
	}{
		{"unicode", Emoji{Name: "👍"}, "👍", "👍"},
		{"custom", Emoji{ID: 1, Name: "void"}, "void:1", "<:void:1>"},
		{"animated", Emoji{ID: 1, Name: "void", Animated: true}, "void:1", "<a:void:1>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.emoji.Reaction(); got != tt.wantReaction {
				t.Errorf("Reaction() = %q, want %q", got, tt.wantReaction)
			}
			if got := tt.emoji.Mention(); got != tt.wantMention {
				t.Errorf("Mention() = %q, want %q", got, tt.wantMention)
			}
		})
	}
}

// TestMessageReaction_Emoji decodes a unicode reaction as Discord sends it, with a null id
func TestMessageReaction_Emoji(t *testing.T) {
	var message Message
	if err := message.UnmarshalJSON([]byte(`{"id":"1","reactions":[{"count":1,"count_details":{"burst":0,"normal":1},"me":false,"me_burst":false,"burst_colors":[],"emoji":{"id":null,"name":"👍"}}]}`)); err != nil {
		t.Fatal(err)
	}
	if got := message.Reactions[0].Emoji.Reaction(); got != "👍" {
		t.Errorf("Reaction() = %q, want %q", got, "👍")
	}
}
