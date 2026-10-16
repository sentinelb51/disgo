package discord

import (
	"testing"
)

// TestUnmarshalComponent_Misplaced decodes components where their type can't be: they must decode as
// UnknownComponent, keeping their data, rather than panic
func TestUnmarshalComponent_Misplaced(t *testing.T) {
	tests := []struct {
		name   string
		decode func() (Component, error)
		want   ComponentType
	}{
		{"button in message", func() (Component, error) {
			var v Message
			err := v.UnmarshalJSON([]byte(`{"id":"1","components":[{"type":2,"id":1,"style":1,"custom_id":"x"}]}`))
			return first(v.Components), err
		}, ComponentTypeButton},
		{"text display in action row", func() (Component, error) {
			var v ActionRowComponent
			err := v.UnmarshalJSON([]byte(`{"type":1,"components":[{"type":10,"id":1,"content":"x"}]}`))
			return first(v.Components), err
		}, ComponentTypeTextDisplay},
		{"button in section", func() (Component, error) {
			var v SectionComponent
			err := v.UnmarshalJSON([]byte(`{"type":9,"components":[{"type":2,"id":1,"style":1,"custom_id":"x"}],"accessory":{"type":11,"media":{"url":"https://x"}}}`))
			return first(v.Components), err
		}, ComponentTypeButton},
		{"text display as accessory", func() (Component, error) {
			var v SectionComponent
			err := v.UnmarshalJSON([]byte(`{"type":9,"components":[{"type":10,"content":"x"}],"accessory":{"type":10,"id":1,"content":"x"}}`))
			return v.Accessory, err
		}, ComponentTypeTextDisplay},
		{"thumbnail in container", func() (Component, error) {
			var v ContainerComponent
			err := v.UnmarshalJSON([]byte(`{"type":17,"components":[{"type":11,"id":1,"media":{"url":"https://x"}}]}`))
			return first(v.Components), err
		}, ComponentTypeThumbnail},
		{"text display in label", func() (Component, error) {
			var v LabelComponent
			err := v.UnmarshalJSON([]byte(`{"type":18,"label":"x","component":{"type":10,"id":1,"content":"x"}}`))
			return v.Component, err
		}, ComponentTypeTextDisplay},
		{"text input in modal", func() (Component, error) {
			var v ModalSubmitInteractionData
			err := v.UnmarshalJSON([]byte(`{"custom_id":"x","components":[{"type":4,"id":1,"custom_id":"x","value":"x"}]}`))
			return first(v.Components), err
		}, ComponentTypeTextInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()

			component, err := tt.decode()
			if err != nil {
				t.Fatal(err)
			}
			unknown, ok := component.(UnknownComponent)
			if !ok {
				t.Fatalf("decoded as %T, want UnknownComponent", component)
			}
			if unknown.Type() != tt.want || unknown.GetID() != 1 || len(unknown.Data) == 0 {
				t.Errorf("decoded as %+v, want type %d, id 1 and its data", unknown, tt.want)
			}
		})
	}
}

func first[T Component](components []T) Component {
	if len(components) == 0 {
		return nil
	}
	return components[0]
}
