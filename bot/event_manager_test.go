package bot

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/gateway"
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

func TestHandleGatewayEventHandlerPanic(t *testing.T) {
	var logged bytes.Buffer
	client := &Client{}
	client.EventManager = NewEventManager(client,
		WithEventManagerLogger(slog.New(slog.NewTextHandler(&logged, nil))),
		WithGatewayHandlers(map[gateway.EventType]GatewayEventHandler{
			gateway.EventTypeGuildCreate: NewGatewayEventHandler(gateway.EventTypeGuildCreate, func(*Client, int, int, gateway.EventGuildCreate) {
				panic("handler failed")
			}),
		}),
	)

	// a panic here would end the gateway's read loop, and with it the process
	client.EventManager.HandleGatewayEvent(gateway.New("token", nil), gateway.EventTypeGuildCreate, 1, gateway.EventGuildCreate{})
	if !strings.Contains(logged.String(), "handler failed") {
		t.Errorf("the panic wasn't logged: %q", logged.String())
	}
}
