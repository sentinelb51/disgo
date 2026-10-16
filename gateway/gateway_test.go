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

// serve plays Discord: HELLO, then READY on IDENTIFY, RESUMED on RESUME and an ACK on every HEARTBEAT
func (c *testConn) serve(heartbeatInterval time.Duration) {
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

	g := newTestGateway(t, func(n int, c *testConn) { c.serve(time.Minute) })
	openTestGateway(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	g.Close(ctx)
}
