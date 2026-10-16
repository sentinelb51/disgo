package bot

import (
	"context"
	"sync"
)

// WaitForEvent waits for an event passing the filterFunc and then calls the actionFunc. You can cancel this function with the passed context.Context and the cancelFunc gets called then.
func WaitForEvent[E Event](client *Client, ctx context.Context, filterFunc func(e E) bool, actionFunc func(e E), cancelFunc func()) {
	ch, cancel := NewEventCollector(client, filterFunc)

	select {
	case <-ctx.Done():
		cancel()
		if cancelFunc != nil {
			cancelFunc()
		}
	case e := <-ch:
		cancel()
		if actionFunc != nil {
			actionFunc(e)
		}
	}
}

// NewEventCollector returns a channel in which the events of type T gets sent which pass the passed filter and a function which can be used to stop the event collector.
// The close function needs to be called to stop the event collector.
func NewEventCollector[E Event](client *Client, filterFunc func(e E) bool) (<-chan E, func()) {
	ch := make(chan E)
	done := make(chan struct{})
	var (
		once sync.Once
		mu   sync.RWMutex
	)

	handler := NewListenerFunc(func(e E) {
		if !filterFunc(e) {
			return
		}
		// ch is closed under mu's write lock, after done
		mu.RLock()
		defer mu.RUnlock()
		select {
		case <-done:
			return
		default:
		}
		select {
		case ch <- e:
		case <-done:
		}
	})
	client.EventManager.AddEventListeners(handler)

	return ch, func() {
		once.Do(func() {
			// release a listener blocked on ch first: a sync dispatch holds the lock RemoveEventListeners waits for
			close(done)
			client.EventManager.RemoveEventListeners(handler)
			mu.Lock()
			close(ch)
			mu.Unlock()
		})
	}
}
