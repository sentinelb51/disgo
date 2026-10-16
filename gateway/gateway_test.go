package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/disgoorg/disgo/discord"
)

// testFrame is a frame as the fake gateway reads and writes it
type testFrame struct {
	Op Opcode          `json:"op"`
	S  int             `json:"s,omitempty"`
	T  EventType       `json:"t,omitempty"`
	D  json.RawMessage `json:"d,omitempty"`
}

// testConn is one connection to the fake gateway
type testConn struct {
	t    *testing.T
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *testConn) write(op Opcode, s int, t EventType, d string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := testFrame{Op: op, S: s, T: t}
	if d != "" {
		frame.D = json.RawMessage(d)
	}
	return c.conn.WriteJSON(frame)
}

func (c *testConn) read() (testFrame, error) {
	var frame testFrame
	err := c.conn.ReadJSON(&frame)
	return frame, err
}

// serve plays Discord: HELLO, then READY on IDENTIFY, RESUMED on RESUME and an ACK on every HEARTBEAT.
// ready, if not nil, runs after READY
func (c *testConn) serve(heartbeatInterval time.Duration, ready func()) {
	if err := c.write(OpcodeHello, 0, "", `{"heartbeat_interval":`+strconv.Itoa(int(heartbeatInterval/time.Millisecond))+`}`); err != nil {
		return
	}
	for {
		frame, err := c.read()
		if err != nil {
			return
		}
		switch frame.Op {
		case OpcodeIdentify:
			err = c.write(OpcodeDispatch, 1, EventTypeReady, `{"v":10,"session_id":"session","resume_gateway_url":"`+c.conn.LocalAddr().String()+`"}`)
			if err == nil && ready != nil {
				ready()
			}
		case OpcodeResume:
			err = c.write(OpcodeDispatch, 0, EventTypeResumed, `{}`)
		case OpcodeHeartbeat:
			err = c.write(OpcodeHeartbeatACK, 0, "", "")
		}
		if err != nil {
			return
		}
	}
}

// newTestGateway starts a fake gateway that hands each connection to handle, and a Gateway that dials it
func newTestGateway(t *testing.T, handle func(n int, c *testConn), opts ...ConfigOpt) Gateway {
	t.Helper()

	var (
		upgrader websocket.Upgrader
		mu       sync.Mutex
		n        int
		wg       sync.WaitGroup
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		mu.Lock()
		i := n
		n++
		mu.Unlock()

		wg.Add(1)
		defer wg.Done()
		defer conn.Close()
		handle(i, &testConn{t: t, conn: conn})
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		wg.Wait()
	})

	opts = append([]ConfigOpt{
		WithURL("ws" + strings.TrimPrefix(server.URL, "http")),
		WithEnableResumeURL(false),
		WithCompression(CompressionNone),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	}, opts...)
	return New("token", func(Gateway, EventType, int, EventData) {}, opts...)
}

func openTestGateway(t *testing.T, g Gateway) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := g.Open(ctx); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		g.Close(ctx)
	})
}

// Close stops the heartbeat started for the connection it closes, which runs on its own goroutine
func TestGatewayCloseStopsHeartbeat(t *testing.T) {
	t.Parallel()

	g := newTestGateway(t, func(n int, c *testConn) { c.serve(time.Minute, nil) })
	openTestGateway(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	g.Close(ctx)
}

// The sequence and session are written by the read loop and read by the caller, the heartbeat and reconnects
func TestGatewaySessionState(t *testing.T) {
	t.Parallel()

	const last = 200
	g := newTestGateway(t, func(n int, c *testConn) {
		c.serve(time.Minute, func() {
			for s := 2; s <= last; s++ {
				if err := c.write(OpcodeDispatch, s, "TEST_EVENT", `{}`); err != nil {
					return
				}
			}
		})
	})
	openTestGateway(t, g)

	deadline := time.Now().Add(5 * time.Second)
	for {
		sequence := g.LastSequenceReceived()
		if sequence != nil && *sequence == last {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last sequence received: %v, want %d", sequence, last)
		}
		time.Sleep(time.Millisecond)
	}
	if sessionID := g.SessionID(); sessionID == nil || *sessionID != "session" {
		t.Fatalf("session id: %v, want session", sessionID)
	}
}

// Latency reads the heartbeat times the heartbeat goroutine and the read loop write
func TestGatewayLatency(t *testing.T) {
	t.Parallel()

	g := newTestGateway(t, func(n int, c *testConn) { c.serve(10*time.Millisecond, nil) })
	openTestGateway(t, g)

	for range 200 {
		_ = g.Latency()
		time.Sleep(time.Millisecond)
	}
}

// slowUnlockRateLimiter lets every command through, but returns from Unlock only a while after the command was
// written, as a busy process may
type slowUnlockRateLimiter struct{ delay time.Duration }

func (slowUnlockRateLimiter) Close(context.Context)                              {}
func (slowUnlockRateLimiter) Reset()                                             {}
func (slowUnlockRateLimiter) Wait(context.Context, RateLimiterCommandType) error { return nil }
func (l slowUnlockRateLimiter) Unlock()                                          { time.Sleep(l.delay) }

// A heartbeat ACK read before sendHeartbeat returns still answers that heartbeat
func TestGatewayHeartbeatACKBeforeSendReturns(t *testing.T) {
	t.Parallel()

	closed := make(chan error, 1)
	g := newTestGateway(t, func(n int, c *testConn) {
		if err := c.write(OpcodeHello, 0, "", `{"heartbeat_interval":100}`); err != nil {
			return
		}
		for {
			frame, err := c.read()
			if err != nil {
				closed <- err
				return
			}
			switch frame.Op {
			case OpcodeIdentify:
				err = c.write(OpcodeDispatch, 1, EventTypeReady, `{"v":10,"session_id":"session"}`)
			case OpcodeHeartbeat:
				err = c.write(OpcodeHeartbeatACK, 0, "", "")
			}
			if err != nil {
				return
			}
		}
	}, WithRateLimiter(slowUnlockRateLimiter{delay: 20 * time.Millisecond}))
	openTestGateway(t, g)

	select {
	case err := <-closed:
		t.Fatalf("gateway closed a healthy connection: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
}

// presenceServer reports the presence of every IDENTIFY, and invalidates the first session on its first PRESENCE_UPDATE,
// so the Gateway identifies again
func presenceServer(identified chan<- string) func(n int, c *testConn) {
	return func(n int, c *testConn) {
		if err := c.write(OpcodeHello, 0, "", `{"heartbeat_interval":60000}`); err != nil {
			return
		}
		invalidated := false
		for {
			frame, err := c.read()
			if err != nil {
				return
			}
			switch frame.Op {
			case OpcodeIdentify:
				var d struct {
					Presence json.RawMessage `json:"presence"`
				}
				_ = json.Unmarshal(frame.D, &d)
				identified <- string(d.Presence)
				err = c.write(OpcodeDispatch, 1, EventTypeReady, `{"v":10,"session_id":"session"}`)
			case OpcodePresenceUpdate:
				if n == 0 && !invalidated {
					invalidated = true
					err = c.write(OpcodeInvalidSession, 0, "", "false")
				}
			}
			if err != nil {
				return
			}
		}
	}
}

func nextIdentify(t *testing.T, identified <-chan string) string {
	t.Helper()

	select {
	case presence := <-identified:
		return presence
	case <-time.After(10 * time.Second):
		t.Fatal("no identify")
		return ""
	}
}

// A presence sent is sent again when the Gateway identifies again
func TestGatewayPresenceKeptForIdentify(t *testing.T) {
	t.Parallel()

	identified := make(chan string, 2)
	g := newTestGateway(t, presenceServer(identified))
	openTestGateway(t, g)
	nextIdentify(t, identified)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.Send(ctx, OpcodePresenceUpdate, MessageDataPresenceUpdate{Status: discord.OnlineStatusDND}); err != nil {
		t.Fatal(err)
	}

	if presence := nextIdentify(t, identified); !strings.Contains(presence, `"status":"dnd"`) {
		t.Fatalf("identify presence: %s, want the dnd status sent", presence)
	}
}

// Changing the presence, as bot.Client.SetPresence does, while the Gateway identifies
func TestGatewayPresenceWhileIdentifying(t *testing.T) {
	t.Parallel()

	identified := make(chan string, 2)
	g := newTestGateway(t, presenceServer(identified), WithPresenceOpts(WithOnlineStatus(discord.OnlineStatusOnline)))
	openTestGateway(t, g)
	nextIdentify(t, identified)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for {
			select {
			case <-done:
				return
			default:
			}
			presence := g.Presence()
			WithOnlineStatus(discord.OnlineStatusIdle)(presence)
			_ = g.Send(ctx, OpcodePresenceUpdate, *presence)
			time.Sleep(time.Millisecond)
		}
	}()
	defer wg.Wait()
	defer close(done)

	if presence := nextIdentify(t, identified); !strings.Contains(presence, `"status":"idle"`) {
		t.Fatalf("identify presence: %s, want the idle status sent", presence)
	}
}
