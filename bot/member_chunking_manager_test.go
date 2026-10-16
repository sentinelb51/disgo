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

// chunkingGateway records the members requests sent through it.
type chunkingGateway struct {
	gateway.Gateway
	nonce string
	err   error
}

func (g *chunkingGateway) Intents() gateway.Intents {
	return gateway.IntentGuildMembers
}

func (g *chunkingGateway) Send(_ context.Context, _ gateway.Opcode, data gateway.MessageData) error {
	g.nonce = data.(gateway.MessageDataRequestGuildMembers).Nonce
	return g.err
}

func newChunkingManager() (*memberChunkingManagerImpl, *chunkingGateway) {
	g := &chunkingGateway{}
	client := &Client{Gateway: g, Caches: cache.New()}
	return NewMemberChunkingManager(client, nil, nil).(*memberChunkingManagerImpl), g
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
