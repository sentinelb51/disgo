package rest

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/discord"
)

var benchMemberBody = []byte(`{"user":{"id":"170939974227591168","username":"test","global_name":"Test","avatar":null,"discriminator":"0","public_flags":0},"nick":null,"avatar":null,"roles":["1234567890123456789"],"joined_at":"2021-01-01T00:00:00.000000+00:00","premium_since":null,"deaf":false,"mute":false,"flags":0,"pending":false}`)

func setBenchRateLimitHeaders(h http.Header) {
	h.Set("Content-Type", "application/json")
	h.Set("Via", "1.1 google")
	h.Set("X-RateLimit-Bucket", "80c17d2f203122d936070c88c8d10f33")
	h.Set("X-RateLimit-Limit", "5")
	h.Set("X-RateLimit-Remaining", "4")
	h.Set("X-RateLimit-Reset", "1760600000.123")
	h.Set("X-RateLimit-Reset-After", "1.000")
}

type benchTransport struct{}

func (benchTransport) RoundTrip(rq *http.Request) (*http.Response, error) {
	if rq.Body != nil {
		_ = rq.Body.Close()
	}
	rs := &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header, 8), Body: io.NopCloser(bytes.NewReader(benchMemberBody)), Request: rq}
	setBenchRateLimitHeaders(rs.Header)
	return rs, nil
}

func benchmarkGetMember(b *testing.B, client Client) {
	guildID := snowflake.ID(81384788765712384)
	userID := snowflake.ID(170939974227591168)
	b.ReportAllocs()
	for b.Loop() {
		var member discord.Member
		if err := client.Do(GetMember.Compile(nil, guildID, userID), nil, &member); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientDo(b *testing.B) {
	b.Run("stub", func(b *testing.B) {
		client := NewClient("token", WithHTTPClient(&http.Client{Transport: benchTransport{}}), WithRateLimiterConfigOpts(WithCleanupInterval(time.Hour)))
		benchmarkGetMember(b, client)
	})
	b.Run("loopback", func(b *testing.B) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setBenchRateLimitHeaders(w.Header())
			_, _ = w.Write(benchMemberBody)
		}))
		defer srv.Close()
		client := NewClient("token", WithURL(srv.URL), WithHTTPClient(srv.Client()), WithRateLimiterConfigOpts(WithCleanupInterval(time.Hour)))
		benchmarkGetMember(b, client)
	})
}
