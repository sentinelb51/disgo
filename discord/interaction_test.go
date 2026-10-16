package discord

import (
	"reflect"
	"testing"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"
)

// TestInteraction_MarshalJSON marshals interactions and decodes them back: the data must survive
func TestInteraction_MarshalJSON(t *testing.T) {
	guildID := snowflake.ID(5)
	targetID := snowflake.ID(4)
	channel := InteractionChannel{MessageChannel: DMChannel{id: 6}}

	tests := []struct {
		name        string
		interaction Interaction
	}{
		{"button", ComponentInteraction{
			baseInteraction: baseInteraction{id: 1, applicationID: 2, token: "token", version: 1, channel: channel},
			Data:            ButtonInteractionData{customID: "verify:start"},
		}},
		{"user command", ApplicationCommandInteraction{
			baseInteraction: baseInteraction{id: 1, applicationID: 2, token: "token", version: 1, channel: channel, guildID: &guildID},
			Data: UserCommandInteractionData{id: 3, name: "info", guildID: &guildID, targetID: targetID, Resolved: UserCommandResolved{
				Users: map[snowflake.ID]User{targetID: {ID: targetID, Username: "jane"}},
			}},
		}},
		{"message command", ApplicationCommandInteraction{
			baseInteraction: baseInteraction{id: 1, applicationID: 2, token: "token", version: 1, channel: channel},
			Data:            MessageCommandInteractionData{id: 3, name: "report", targetID: targetID},
		}},
		{"entry point command", ApplicationCommandInteraction{
			baseInteraction: baseInteraction{id: 1, applicationID: 2, token: "token", version: 1, channel: channel},
			Data:            EntryPointCommandInteractionData{id: 3, name: "launch"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.interaction)
			if err != nil {
				t.Fatal(err)
			}

			decoded, err := UnmarshalInteraction(data)
			if err != nil {
				t.Fatalf("%v: %s", err, data)
			}

			var want, got any
			switch interaction := tt.interaction.(type) {
			case ComponentInteraction:
				want, got = interaction.Data, decoded.(ComponentInteraction).Data
			case ApplicationCommandInteraction:
				want, got = interaction.Data, decoded.(ApplicationCommandInteraction).Data
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %+v, want %+v from %s", got, want, data)
			}
		})
	}
}
