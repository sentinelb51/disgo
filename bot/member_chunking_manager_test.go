package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

// chunkingGateway records the members requests sent through it, on session "s1" of shard 0, also on sent if set.
type chunkingGateway struct {
	gateway.Gateway
	nonce   string
	err     error
	intents gateway.Intents
	sent    chan string
}

func (g *chunkingGateway) Intents() gateway.Intents {
	return g.intents
}

func (g *chunkingGateway) ShardID() int {
	return 0
}

func (g *chunkingGateway) SessionID() *string {
	session := "s1"
	return &session
}

func (g *chunkingGateway) Send(_ context.Context, _ gateway.Opcode, data gateway.MessageData) error {
	g.nonce = data.(gateway.MessageDataRequestGuildMembers).Nonce
	if g.sent != nil {
		g.sent <- g.nonce
	}
	return g.err
}

func newChunkingManager() (*memberChunkingManagerImpl, *chunkingGateway) {
	g := &chunkingGateway{intents: gateway.IntentGuildMembers}
	client := &Client{Gateway: g, Caches: cache.New()}
	return NewMemberChunkingManager(client, nil, nil).(*memberChunkingManagerImpl), g
}

// requestAsync requests members by ID as RequestMembers does, waiting on its own goroutine, and returns the
// request's nonce once sent and where its result comes
func requestAsync(m *memberChunkingManagerImpl, g *chunkingGateway) (string, <-chan error) {
	g.sent = make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		_, err := m.RequestMembers(context.Background(), 1, 2, 3)
		done <- err
	}()
	return <-g.sent, done
}

func waitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the request is still waiting")
		return nil
	}
}

// A request Discord rate-limits gets no chunks: it fails at once, where it waited out its context, forever for
// context.Background
func TestMemberChunkingRateLimited(t *testing.T) {
	m, g := newChunkingManager()

	nonce, done := requestAsync(m, g)
	m.HandleRateLimited(gateway.EventRateLimited{Opcode: gateway.OpcodeRequestGuildMembers, RetryAfter: 1.5,
		Meta: gateway.RateLimitedMetadataRequestGuildMembers{GuildID: 1, Nonce: "another"}})
	m.HandleRateLimited(gateway.EventRateLimited{Opcode: gateway.OpcodeRequestGuildMembers, RetryAfter: 1.5,
		Meta: gateway.RateLimitedMetadataRequestGuildMembers{GuildID: 1, Nonce: nonce}})

	if err := waitErr(t, done); !errors.Is(err, ErrMemberRequestRateLimited) || err.Error() != "member request rate limited: retry after 1.5s" {
		t.Fatalf("err = %v", err)
	}
	if len(m.chunkingRequests) != 0 {
		t.Error("rate-limited request still registered")
	}
}

// A new session of the request's shard sends no chunks for it; the session's own READY, or another shard's, ends
// nothing
func TestMemberChunkingNewSession(t *testing.T) {
	m, g := newChunkingManager()

	_, done := requestAsync(m, g)
	m.HandleReady(0, "s1")
	m.HandleReady(1, "s2")
	select {
	case err := <-done:
		t.Fatalf("ended by a READY not its shard's new session: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	m.HandleReady(0, "s2")
	if err := waitErr(t, done); !errors.Is(err, ErrMemberRequestSessionEnded) {
		t.Fatalf("err = %v", err)
	}
}

func chunk(nonce string, count int, members ...discord.Member) gateway.EventGuildMembersChunk {
	return gateway.EventGuildMembersChunk{GuildID: 1, Members: members, ChunkCount: count, Nonce: nonce}
}

func TestMemberChunkingCancelAfterLastChunk(t *testing.T) {
	m, g := newChunkingManager()

	members, cancel, err := m.RequestMembersChan(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	go m.HandleChunk(chunk(g.nonce, 1, discord.Member{User: discord.User{ID: 2}}))
	for range members {
	}

	// the channel is closed already; cancelling must not close it again
	cancel()
}

func TestMemberChunkingCancelDuringChunk(t *testing.T) {
	m, g := newChunkingManager()

	members, cancel, err := m.RequestAllMembersChan(1)
	if err != nil {
		t.Fatal(err)
	}

	handled := make(chan struct{})
	go func() {
		defer close(handled)
		m.HandleChunk(chunk(g.nonce, 2, discord.Member{User: discord.User{ID: 2}}, discord.Member{User: discord.User{ID: 3}}))
	}()

	// take the first member, then cancel while HandleChunk holds the second
	<-members
	cancel()

	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleChunk still blocked after the request was cancelled")
	}
	if _, ok := <-members; ok {
		t.Error("member channel still open after cancel")
	}
	if len(m.chunkingRequests) != 0 {
		t.Error("cancelled request still registered")
	}

	// the request's last chunk arrives after it was cancelled
	m.HandleChunk(chunk(g.nonce, 2))
}

func TestMemberChunkingSendError(t *testing.T) {
	m, g := newChunkingManager()
	g.err = errors.New("not sent")

	if _, err := m.RequestMembers(context.Background(), 1, 2); !errors.Is(err, g.err) {
		t.Fatalf("err = %v, want %v", err, g.err)
	}
	if len(m.chunkingRequests) != 0 {
		t.Error("request that was never sent is still registered")
	}
}
