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

// Run with -race: Unlock records a global rate limit while requests on other buckets read it in Wait.
func TestRateLimiterGlobalConcurrentAccess(t *testing.T) {
	l := NewRateLimiter(WithCleanupInterval(time.Hour))
	global := GetGatewayBot.Compile(nil)
	other := GetCurrentUser.Compile(nil)

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			if err := l.Wait(context.Background(), global); err != nil {
				t.Error(err)
				return
			}
			_ = l.Unlock(global, newRateLimitResponse(http.StatusTooManyRequests, map[string]string{
				"Retry-After":        "0",
				"X-RateLimit-Global": "true",
			}))
		}
	})
	wg.Go(func() {
		for range 100 {
			if err := l.Wait(context.Background(), other); err != nil {
				t.Error(err)
				return
			}
			_ = l.Unlock(other, nil)
		}
	})
	wg.Wait()
}

func TestRateLimiterExhaustedBucketWaitsForGlobal(t *testing.T) {
	l := NewRateLimiter(WithCleanupInterval(time.Hour))
	exhausted := GetGatewayBot.Compile(nil)
	global := GetCurrentUser.Compile(nil)

	if err := l.Wait(context.Background(), exhausted); err != nil {
		t.Fatal(err)
	}
	if err := l.Unlock(exhausted, newRateLimitResponse(http.StatusOK, map[string]string{
		"X-RateLimit-Bucket":      "abcd1234",
		"X-RateLimit-Limit":       "5",
		"X-RateLimit-Remaining":   "0",
		"X-RateLimit-Reset-After": "0.2",
	})); err != nil {
		t.Fatal(err)
	}

	if err := l.Wait(context.Background(), global); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Unlock(global, newRateLimitResponse(http.StatusTooManyRequests, map[string]string{
		"Retry-After":        "1",
		"X-RateLimit-Global": "true",
	})); err != nil {
		t.Fatal(err)
	}

	if err := l.Wait(context.Background(), exhausted); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 900*time.Millisecond {
		t.Errorf("request went out %s into a 1s global rate limit", waited)
	}
	_ = l.Unlock(exhausted, nil)
}

// Run with -race: one request updates its bucket in Unlock while the next one waits for that bucket.
func TestRateLimiterBucketConcurrentAccess(t *testing.T) {
	l := NewRateLimiter(WithCleanupInterval(time.Hour))
	endpoint := GetGatewayBot.Compile(nil)

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for range 100 {
				if err := l.Wait(context.Background(), endpoint); err != nil {
					t.Error(err)
					return
				}
				_ = l.Unlock(endpoint, newRateLimitResponse(http.StatusOK, map[string]string{
					"X-RateLimit-Bucket":      "abcd1234",
					"X-RateLimit-Limit":       "5",
					"X-RateLimit-Remaining":   "4",
					"X-RateLimit-Reset-After": "1",
				}))
			}
		})
	}
	wg.Wait()
}

func TestClientCloseWaitsForPendingRequest(t *testing.T) {
	received := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(received)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient("token", WithURL(srv.URL), WithHTTPClient(srv.Client()), WithRateLimiterConfigOpts(WithCleanupInterval(time.Hour)))
	done := make(chan error, 1)
	go func() {
		done <- client.Do(GetGatewayBot.Compile(nil), nil, nil)
	}()
	<-received

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	closed := make(chan struct{})
	go func() {
		client.Close(ctx)
		close(closed)
	}()
	time.Sleep(100 * time.Millisecond)
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request did not finish while Close waited for it")
	}
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("Close did not return once the pending request finished")
	}
}
