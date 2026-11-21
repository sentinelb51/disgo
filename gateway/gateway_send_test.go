package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/disgoorg/disgo/discord"
)

// writtenTransport records what is written to it; anything else it isn't asked
type writtenTransport struct {
	transport
	written []Opcode
}

func (t *writtenTransport) WriteMessage(message Message) error {
	t.written = append(t.written, message.Op)
	return nil
}

// heldLimiter holds a Wait until released, then fails it with err
type heldLimiter struct {
	waiting, release chan struct{}
	err              error
}

func (l *heldLimiter) Close(context.Context) {}
func (l *heldLimiter) Unlock()               {}
func (l *heldLimiter) Wait(context.Context, RateLimiterCommandType) error {
	l.waiting <- struct{}{}
	<-l.release
	return l.err
}

func sendGateway(status Status, limiter RateLimiter) (*gatewayImpl, *writtenTransport) {
	g := New("token", nil).(*gatewayImpl)
	conn := &writtenTransport{}
	g.conn, g.status, g.config.RateLimiter = conn, status, limiter
	return g, conn
}

// A heartbeat whose wait ran out while its connection was replaced reports the connection gone, which the heartbeat
// ignores, where its timeout made it close the new connection
func TestSendWaitOutlivesConnection(t *testing.T) {
	limiter := &heldLimiter{waiting: make(chan struct{}), release: make(chan struct{}), err: context.DeadlineExceeded}
	g, _ := sendGateway(StatusReady, limiter)

	sent := make(chan error, 1)
	go func() {
		sent <- g.sendInternal(context.Background(), InternalCommandType, OpcodeHeartbeat, MessageDataHeartbeat(1))
	}()

	<-limiter.waiting
	g.connMu.Lock()
	g.conn = &writtenTransport{}
	g.connMu.Unlock()
	close(limiter.release)

	if err := <-sent; !errors.Is(err, discord.ErrShardNotConnected) {
		t.Errorf("sent: %v", err)
	}
}
