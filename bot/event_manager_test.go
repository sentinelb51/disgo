package bot

import (
	"io"
	"log/slog"
	"testing"
)

func TestDispatchEventListenerPanic(t *testing.T) {
	var received bool
	client := &Client{}
	client.EventManager = NewEventManager(client,
		WithEventManagerLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		WithListenerFunc(func(*testEvent) { panic("listener failed") }),
		WithListenerFunc(func(*testEvent) { received = true }),
	)

	client.EventManager.DispatchEvent(&testEvent{})
	if !received {
		t.Error("a listener's panic kept the event from the next listener")
	}
}
