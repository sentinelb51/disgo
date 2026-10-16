package bot

import (
	"testing"
	"time"
)

type testEvent struct{}

func (*testEvent) Client() *Client     { return nil }
func (*testEvent) SequenceNumber() int { return 0 }

// newCollector returns a collector whose filter reports each event it lets through on entered.
func newCollector(client *Client) (<-chan *testEvent, func(), chan struct{}) {
	entered := make(chan struct{}, 1)
	ch, cancel := NewEventCollector(client, func(*testEvent) bool {
		entered <- struct{}{}
		return true
	})
	return ch, cancel, entered
}

func TestEventCollectorCancelWhileDispatching(t *testing.T) {
	client := &Client{}
	client.EventManager = NewEventManager(client)
	_, cancel, entered := newCollector(client)

	go client.EventManager.DispatchEvent(&testEvent{})
	<-entered

	// nobody receives the event; cancelling must release the dispatcher instead of waiting for it
	cancelled := make(chan struct{})
	go func() {
		defer close(cancelled)
		cancel()
	}()

	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel and DispatchEvent deadlocked")
	}
}

func TestEventCollectorCancelWhileSending(t *testing.T) {
	client := &Client{}
	client.EventManager = NewEventManager(client)
	_, cancel, entered := newCollector(client)
	listener := client.EventManager.(*eventManagerImpl).eventListeners[0]

	// an async dispatch calls the listener on a goroutine of its own, without the event manager's lock
	var recovered any
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		defer func() { recovered = recover() }()
		listener.OnEvent(&testEvent{})
	}()
	<-entered

	cancel()
	<-sent
	if recovered != nil {
		t.Fatalf("listener panicked: %v", recovered)
	}
}
