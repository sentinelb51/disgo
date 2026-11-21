package bot

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

var _ MemberChunkingManager = (*memberChunkingManagerImpl)(nil)

// ErrNoUserIDs is returned when no user IDs are provided to request members.
var ErrNoUserIDs = errors.New("no user ids to request")

var (
	// ErrMemberRequestRateLimited is a member request Discord rate-limited: it sends no members for it
	ErrMemberRequestRateLimited = errors.New("member request rate limited")
	// ErrMemberRequestSessionEnded is a member request whose gateway session ended before its members came: a new
	// session sends none for it
	ErrMemberRequestSessionEnded = errors.New("member request's session ended")
)

// NewMemberChunkingManager returns a new MemberChunkingManager with the given MemberChunkingFilter.
func NewMemberChunkingManager(client *Client, logger *slog.Logger, memberChunkingFilter MemberChunkingFilter) MemberChunkingManager {
	if memberChunkingFilter == nil {
		memberChunkingFilter = MemberChunkingFilterNone
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("name", "bot_member_chunking_manager"))

	return &memberChunkingManagerImpl{
		client:               client,
		logger:               logger,
		memberChunkingFilter: memberChunkingFilter,
		chunkingRequests:     map[string]*chunkingRequest{},
	}
}

// MemberChunkingManager is used to request members for guilds from the discord Gateway.
//
// The members come on the gateway's reader, which calls event listeners, by default one at a time on itself: a
// listener requesting members waits on itself (until its context ends, or for ever). Request on a goroutine of
// your own, or enable asynchronous events.
type MemberChunkingManager interface {
	// MemberChunkingFilter returns the configured MemberChunkingFilter used by this MemberChunkingManager.
	MemberChunkingFilter() MemberChunkingFilter

	// HandleChunk handles the discord.EventGuildMembersChunk event payloads from the discord Gateway.
	HandleChunk(payload gateway.EventGuildMembersChunk)

	// HandleRateLimited fails the member request a gateway.EventRateLimited names: Discord sends no members for it.
	HandleRateLimited(payload gateway.EventRateLimited)

	// HandleReady fails the member requests sent on an earlier session of the shard: a new session sends no
	// members for them.
	HandleReady(shardID int, sessionID string)

	// RequestMembers requests members from the given guildID and userIDs, at most 100 of them.
	// It fails with ErrMemberRequestRateLimited or ErrMemberRequestSessionEnded when Discord will send none, and
	// with ctx's error, beside the members already received, when ctx ends first.
	RequestMembers(ctx context.Context, guildID snowflake.ID, userIDs ...snowflake.ID) ([]discord.Member, error)
	// RequestAllMembers requests all members from the given guildID.
	// Notice: This action requires the gateway.IntentGuildMembers.
	RequestAllMembers(ctx context.Context, guildID snowflake.ID) ([]discord.Member, error)
	// RequestMembersWithQuery requests members from the given guildID and query.
	// query : string the username starts with
	// Notice: An empty query, every member, requires the gateway.IntentGuildMembers.
	RequestMembersWithQuery(ctx context.Context, guildID snowflake.ID, query string, limit int) ([]discord.Member, error)
	// RequestMembersWithFilter requests members from the given guildID and userIDs. memberFilterFunc is used to filter all returned members.
	// Notice: This action requires the gateway.IntentGuildMembers.
	RequestMembersWithFilter(ctx context.Context, guildID snowflake.ID, memberFilterFunc func(member discord.Member) bool) ([]discord.Member, error)

	// RequestMembersChan requests members from the given guildID and userIDs.
	// Returns a channel which will receive the members.
	// Returns a function which can be used to cancel the request and close the channel.
	RequestMembersChan(guildID snowflake.ID, userIDs ...snowflake.ID) (<-chan discord.Member, func(), error)
	// RequestAllMembersChan requests all members from the given guildID.
	// Returns a channel which will receive the members.
	// Returns a function which can be used to cancel the request and close the channel.
	// Notice: This action requires the gateway.IntentGuildMembers.
	RequestAllMembersChan(guildID snowflake.ID) (<-chan discord.Member, func(), error)
	// RequestMembersWithQueryChan requests members from the given guildID and query.
	// Returns a channel which will receive the members.
	// Returns a function which can be used to cancel the request and close the channel.
	// Notice: An empty query, every member, requires the gateway.IntentGuildMembers.
	RequestMembersWithQueryChan(guildID snowflake.ID, query string, limit int) (<-chan discord.Member, func(), error)
	// RequestMembersWithFilterChan requests members from the given guildID and userIDs. memberFilterFunc is used to filter all returned members.
	// Returns a channel which will receive the members.
	// Returns a function which can be used to cancel the request and close the channel.
	// Notice: This action requires the gateway.IntentGuildMembers.
	RequestMembersWithFilterChan(guildID snowflake.ID, memberFilterFunc func(member discord.Member) bool) (<-chan discord.Member, func(), error)
}

type chunkingRequest struct {
	sync.Mutex
	nonce string

	// shardID and sessionID are the session the request went out on: a new one sends no members for it
	shardID   int
	sessionID string

	memberChan       chan<- discord.Member
	memberFilterFunc func(member discord.Member) bool

	chunks int
	closed bool
	// err is why the request ended before its last chunk, set before memberChan closes
	err error

	// cancelled is closed on cancel, to stop HandleChunk waiting to send on memberChan
	cancelled  chan struct{}
	cancelOnce sync.Once
}

type memberChunkingManagerImpl struct {
	client               *Client
	logger               *slog.Logger
	memberChunkingFilter MemberChunkingFilter

	chunkingRequestsMu sync.RWMutex
	chunkingRequests   map[string]*chunkingRequest
}

func (m *memberChunkingManagerImpl) MemberChunkingFilter() MemberChunkingFilter {
	return m.memberChunkingFilter
}

func (m *memberChunkingManagerImpl) HandleChunk(payload gateway.EventGuildMembersChunk) {
	m.chunkingRequestsMu.RLock()
	request, ok := m.chunkingRequests[payload.Nonce]
	m.chunkingRequestsMu.RUnlock()
	if !ok {
		m.logger.Debug("received unknown member chunk event", slog.Any("payload", payload))
		return
	}

	request.Lock()
	defer request.Unlock()
	if request.closed {
		return
	}

	delivering := true
	for _, member := range payload.Members {
		// try to cache member
		m.client.Caches.AddMember(member)
		if !delivering || request.memberFilterFunc != nil && !request.memberFilterFunc(member) {
			continue
		}
		select {
		case request.memberChan <- member:
		case <-request.cancelled:
			delivering = false
		}
	}
	// Cancelled or failed while handing a member over: whoever ended it cleans up, with its error
	if !delivering {
		return
	}

	// all chunks sent cleanup
	if request.chunks == payload.ChunkCount-1 {
		cleanupRequest(m, request)
		return
	}
	request.chunks++
}

// cleanupRequest closes the request's member channel and forgets the request. The caller must hold the request's lock.
func cleanupRequest(m *memberChunkingManagerImpl, request *chunkingRequest) {
	if request.closed {
		return
	}
	request.closed = true
	close(request.memberChan)
	m.chunkingRequestsMu.Lock()
	delete(m.chunkingRequests, request.nonce)
	m.chunkingRequestsMu.Unlock()
}

// failRequest ends a request Discord will send no more members for, with err. Waiting on the context was all
// that ended one before. It wakes a HandleChunk blocked handing a member to a reader not reading first: that one
// holds the lock, on the old connection's reader, and the new one's READY would wait for it forever
func failRequest(m *memberChunkingManagerImpl, request *chunkingRequest, err error) {
	request.cancelOnce.Do(func() { close(request.cancelled) })
	request.Lock()
	defer request.Unlock()
	if request.closed {
		return
	}
	request.err = err
	cleanupRequest(m, request)
}

func (m *memberChunkingManagerImpl) HandleRateLimited(payload gateway.EventRateLimited) {
	meta, ok := payload.Meta.(gateway.RateLimitedMetadataRequestGuildMembers)
	if !ok {
		return
	}

	m.chunkingRequestsMu.RLock()
	request, ok := m.chunkingRequests[meta.Nonce]
	m.chunkingRequestsMu.RUnlock()
	if !ok {
		return
	}

	retryAfter := time.Duration(payload.RetryAfter * float64(time.Second))
	failRequest(m, request, fmt.Errorf("%w: retry after %s", ErrMemberRequestRateLimited, retryAfter))
}

func (m *memberChunkingManagerImpl) HandleReady(shardID int, sessionID string) {
	var ended []*chunkingRequest
	m.chunkingRequestsMu.RLock()
	for _, request := range m.chunkingRequests {
		if request.shardID == shardID && request.sessionID != sessionID {
			ended = append(ended, request)
		}
	}
	m.chunkingRequestsMu.RUnlock()

	for _, request := range ended {
		failRequest(m, request, ErrMemberRequestSessionEnded)
	}
}

func (m *memberChunkingManagerImpl) requestGuildMembersChan(ctx context.Context, guildID snowflake.ID, query *string, limit *int, userIDs []snowflake.ID, memberFilterFunc func(member discord.Member) bool) (<-chan discord.Member, func(), error) {
	_, memberChan, cancel, err := m.startRequest(ctx, guildID, query, limit, userIDs, memberFilterFunc)
	return memberChan, cancel, err
}

// startRequest sends a member request, and returns it with the channel its members come on, closed once they're
// all in or the request ends
func (m *memberChunkingManagerImpl) startRequest(ctx context.Context, guildID snowflake.ID, query *string, limit *int, userIDs []snowflake.ID, memberFilterFunc func(member discord.Member) bool) (*chunkingRequest, <-chan discord.Member, func(), error) {
	shard, err := m.client.Shard(guildID)
	if err != nil {
		return nil, nil, nil, err
	}

	// Only every member, an empty query, takes the intent: Discord sends members by ID or by a username's start
	// without it
	if query != nil && *query == "" && shard.Intents().Missing(gateway.IntentGuildMembers) {
		return nil, nil, nil, discord.ErrNoGuildMembersIntent
	}

	var sessionID string
	if id := shard.SessionID(); id != nil {
		sessionID = *id
	}

	var nonce string
	for {
		nonce = rand.Text()
		m.chunkingRequestsMu.RLock()
		_, ok := m.chunkingRequests[nonce]
		m.chunkingRequestsMu.RUnlock()
		if !ok {
			break
		}
	}
	memberChan := make(chan discord.Member)
	request := &chunkingRequest{
		nonce:            nonce,
		shardID:          shard.ShardID(),
		sessionID:        sessionID,
		memberChan:       memberChan,
		memberFilterFunc: memberFilterFunc,
		cancelled:        make(chan struct{}),
	}

	m.chunkingRequestsMu.Lock()
	m.chunkingRequests[nonce] = request
	m.chunkingRequestsMu.Unlock()

	command := gateway.MessageDataRequestGuildMembers{
		GuildID:   guildID,
		Query:     query,
		Limit:     limit,
		Presences: shard.Intents().Has(gateway.IntentGuildPresences),
		UserIDs:   userIDs,
		Nonce:     nonce,
	}

	cancel := func() {
		request.cancelOnce.Do(func() { close(request.cancelled) })
		request.Lock()
		defer request.Unlock()
		cleanupRequest(m, request)
	}
	if err = shard.Send(ctx, gateway.OpcodeRequestGuildMembers, command); err != nil {
		// no chunks will come for a request that was never sent
		cancel()
	}
	return request, memberChan, cancel, err
}

func (m *memberChunkingManagerImpl) requestGuildMembers(ctx context.Context, guildID snowflake.ID, query *string, limit *int, userIDs []snowflake.ID, memberFilterFunc func(member discord.Member) bool) ([]discord.Member, error) {
	var members []discord.Member
	request, memberChan, cls, err := m.startRequest(ctx, guildID, query, limit, userIDs, memberFilterFunc)
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			cls()
			// what came before the end is kept: a caller falling back to another way reads only the rest
			return members, ctx.Err()
		case member, ok := <-memberChan:
			if !ok {
				request.Lock()
				err = request.err
				request.Unlock()
				return members, err
			}
			members = append(members, member)
		}
	}
}

func (m *memberChunkingManagerImpl) RequestMembers(ctx context.Context, guildID snowflake.ID, userIDs ...snowflake.ID) ([]discord.Member, error) {
	if len(userIDs) == 0 {
		return nil, ErrNoUserIDs
	}
	return m.requestGuildMembers(ctx, guildID, nil, nil, userIDs, nil)
}

func (m *memberChunkingManagerImpl) RequestMembersWithQuery(ctx context.Context, guildID snowflake.ID, query string, limit int) ([]discord.Member, error) {
	return m.requestGuildMembers(ctx, guildID, &query, &limit, nil, nil)
}

func (m *memberChunkingManagerImpl) RequestAllMembers(ctx context.Context, guildID snowflake.ID) ([]discord.Member, error) {
	query := ""
	limit := 0
	return m.requestGuildMembers(ctx, guildID, &query, &limit, nil, nil)
}

func (m *memberChunkingManagerImpl) RequestMembersWithFilter(ctx context.Context, guildID snowflake.ID, memberFilterFunc func(member discord.Member) bool) ([]discord.Member, error) {
	query := ""
	limit := 0
	return m.requestGuildMembers(ctx, guildID, &query, &limit, nil, memberFilterFunc)
}

func (m *memberChunkingManagerImpl) RequestMembersChan(guildID snowflake.ID, userIDs ...snowflake.ID) (<-chan discord.Member, func(), error) {
	if len(userIDs) == 0 {
		return nil, nil, ErrNoUserIDs
	}
	return m.requestGuildMembersChan(context.Background(), guildID, nil, nil, userIDs, nil)
}

func (m *memberChunkingManagerImpl) RequestMembersWithQueryChan(guildID snowflake.ID, query string, limit int) (<-chan discord.Member, func(), error) {
	return m.requestGuildMembersChan(context.Background(), guildID, &query, &limit, nil, nil)
}

func (m *memberChunkingManagerImpl) RequestAllMembersChan(guildID snowflake.ID) (<-chan discord.Member, func(), error) {
	query := ""
	limit := 0
	return m.requestGuildMembersChan(context.Background(), guildID, &query, &limit, nil, nil)
}

func (m *memberChunkingManagerImpl) RequestMembersWithFilterChan(guildID snowflake.ID, memberFilterFunc func(member discord.Member) bool) (<-chan discord.Member, func(), error) {
	query := ""
	limit := 0
	return m.requestGuildMembersChan(context.Background(), guildID, &query, &limit, nil, memberFilterFunc)
}
