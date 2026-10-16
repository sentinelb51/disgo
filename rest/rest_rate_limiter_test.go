package rest

import (
	"context"
	"net/http"
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
