package gateway

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Senders waiting out the window together: each read the window's end after unlocking, racing the write of the
// first to wake and start the next window (go test -race)
func TestRateLimiterWaitAcrossWindows(t *testing.T) {
	const senders = 4
	l := NewRateLimiter(WithCommandsPerMinute(senders), WithReservedCommandSlots(0)).(*rateLimiterImpl)
	l.reset, l.remaining = time.Now().Add(20*time.Millisecond), 0

	var sent sync.WaitGroup
	for range senders {
		sent.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := l.Wait(ctx, NormalCommandType); err != nil {
				t.Error(err)
				return
			}
			l.Unlock()
		})
	}
	sent.Wait()
}
