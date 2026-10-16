package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newRateLimitResponse(status int, headers map[string]string) *http.Response {
	rs := &http.Response{StatusCode: status, Header: http.Header{}}
	for k, v := range headers {
		rs.Header.Set(k, v)
	}
	return rs
}

func TestRateLimiterFractionalResetAfter(t *testing.T) {
	l := NewRateLimiter(WithCleanupInterval(time.Hour))
	endpoint := GetGatewayBot.Compile(nil)

	if err := l.Wait(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Unlock(endpoint, newRateLimitResponse(http.StatusOK, map[string]string{
		"X-RateLimit-Bucket":      "abcd1234",
		"X-RateLimit-Limit":       "5",
		"X-RateLimit-Remaining":   "0",
		"X-RateLimit-Reset-After": "0.75",
	})); err != nil {
		t.Fatal(err)
	}

	if err := l.Wait(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 700*time.Millisecond {
		t.Errorf("next request on an exhausted bucket went out after %s, want at least the 0.75s of X-RateLimit-Reset-After", waited)
	}
	_ = l.Unlock(endpoint, nil)
}

func TestClientWaitsOnRateLimitWithoutBucket(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			// Discord's documented global rate limit response carries no X-RateLimit-Bucket.
			name: "global",
			headers: map[string]string{
				"Retry-After":        "1",
				"X-RateLimit-Global": "true",
				"X-RateLimit-Scope":  "global",
				"Via":                "1.1 google",
			},
		},
		{
			// Cloudflare's 429s carry none of Discord's headers.
			name: "cloudflare",
			headers: map[string]string{
				"Retry-After": "1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				mu    sync.Mutex
				times []time.Time
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				times = append(times, time.Now())
				first := len(times) == 1
				mu.Unlock()
				if first {
					for k, v := range tt.headers {
						w.Header().Set(k, v)
					}
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"message": "You are being rate limited.", "retry_after": 0.9, "global": true}`))
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			client := NewClient("token", WithURL(srv.URL), WithHTTPClient(srv.Client()), WithRateLimiterConfigOpts(WithCleanupInterval(time.Hour)))
			if err := client.Do(GetGatewayBot.Compile(nil), nil, nil); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(times) != 2 {
				t.Fatalf("got %d requests, want 2", len(times))
			}
			if waited := times[1].Sub(times[0]); waited < 900*time.Millisecond {
				t.Errorf("retried after %s, want at least the 1s of Retry-After", waited)
			}
		})
	}
}
