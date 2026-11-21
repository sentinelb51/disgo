package gateway

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// BenchmarkReceive times a transport receiving MESSAGE_CREATE frames over a loopback websocket, compressed as one
// stream as Discord compresses them: reading, inflating and decoding
func BenchmarkReceive(b *testing.B) {

	indented, err := os.ReadFile("testdata/frames/message_create.json")
	if err != nil {
		b.Fatal(err)
	}
	var frame bytes.Buffer
	if err = json.Compact(&frame, indented); err != nil {
		b.Fatal(err)
	}

	for _, compression := range []CompressionType{CompressionZstdStream, CompressionZlibStream} {
		b.Run(compression.String(), func(b *testing.B) {

			compress := streamCompressor(b, compression)
			frames := make([][]byte, b.N)
			for i := range frames {
				frames[i] = compress(frame.Bytes())
			}

			var upgrader websocket.Upgrader
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				for _, compressed := range frames {
					if conn.WriteMessage(websocket.BinaryMessage, compressed) != nil {
						return
					}
				}
				_, _, _ = conn.ReadMessage()
			}))
			defer server.Close()

			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				b.Fatal(err)
			}
			defer conn.Close()

			receiver := newTransport(compression, conn, slog.New(slog.DiscardHandler), false)
			b.SetBytes(int64(frame.Len()))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				message, err := receiver.ReceiveMessage()
				if err != nil || message == nil || message.T != EventTypeMessageCreate {
					b.Fatalf("%v, %v", message, err)
				}
			}
		})
	}
}
