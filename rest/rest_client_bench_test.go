package rest

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// benchTransport answers every request with body, a member's by default
type benchTransport struct {
	body []byte
}

func (t benchTransport) RoundTrip(rq *http.Request) (*http.Response, error) {
	if rq.Body != nil {
		_ = rq.Body.Close()
	}
	body := t.body
	if body == nil {
		body = benchMemberBody
	}
	rs := &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header, 8), Body: io.NopCloser(bytes.NewReader(body)), Request: rq}
	setBenchRateLimitHeaders(rs.Header)
	return rs, nil
}

// benchMessage is a message as GET /channels/{id}/messages lists it: a user's reply, with a mention and a link embed
const benchMessage = `{"type":19,"content":"yeah <@402938475610293847> that's the one, the server boost ran out so the emojis are gone https://example.com/post","mentions":[{"id":"402938475610293847","username":"modjane","avatar":"1d2c3b4a59687f6e5d4c3b2a19087f6e","discriminator":"0","public_flags":0,"flags":0,"banner":null,"accent_color":null,"global_name":"Jane","avatar_decoration_data":null,"collectibles":null,"display_name_styles":null,"banner_color":null,"clan":null,"primary_guild":null}],"mention_roles":[],"attachments":[],"embeds":[{"type":"article","url":"https://example.com/post","title":"A post","description":"Something someone wrote about something else, long enough to be cut short in the client.","color":2105893,"provider":{"name":"Example"},"thumbnail":{"url":"https://example.com/thumb.png","proxy_url":"https://images-ext-1.discordapp.net/external/abc/https/example.com/thumb.png","width":400,"height":209,"placeholder":"3vcJDYIIdniIaXd4eHiId4eHePdpd5gP","placeholder_version":1,"flags":0},"content_scan_version":2}],"timestamp":"2026-09-30T22:14:05.512000+00:00","edited_timestamp":null,"flags":0,"components":[],"id":"1422876543210987999","channel_id":"1102938475610293845","author":{"id":"912345678901234567","username":"someone","avatar":"a1b2c3d4e5f60718293a4b5c6d7e8f90","discriminator":"0","public_flags":0,"flags":0,"banner":null,"accent_color":null,"global_name":"Someone","avatar_decoration_data":null,"collectibles":null,"display_name_styles":null,"banner_color":null,"clan":null,"primary_guild":null},"pinned":false,"mention_everyone":false,"tts":false,"message_reference":{"type":0,"channel_id":"1102938475610293845","message_id":"1422876543210987000","guild_id":"1102938475610293840"},"position":0}`

// benchMessagesBody is a full page of messages
var benchMessagesBody = []byte("[" + strings.Repeat(benchMessage+",", 99) + benchMessage + "]")

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
	b.Run("messages", func(b *testing.B) {
		client := NewClient("token", WithHTTPClient(&http.Client{Transport: benchTransport{body: benchMessagesBody}}), WithRateLimiterConfigOpts(WithCleanupInterval(time.Hour)))
		endpoint := GetMessages.Compile(nil, snowflake.ID(1102938475610293845))
		b.SetBytes(int64(len(benchMessagesBody)))
		b.ReportAllocs()
		for b.Loop() {
			var messages []discord.Message
			if err := client.Do(endpoint, nil, &messages); err != nil {
				b.Fatal(err)
			}
		}
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
