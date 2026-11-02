package cache

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/discord"
)

func textChannel(t testing.TB, id, guildID snowflake.ID) discord.GuildChannel {
	t.Helper()
	return guildChannel(t, fmt.Sprintf(`{"id":"%d","type":0,"guild_id":"%d"}`, id, guildID))
}

func channelIDs(caches Caches, guildID snowflake.ID) []snowflake.ID {
	var ids []snowflake.ID
	for channel := range caches.ChannelsForGuild(guildID) {
		ids = append(ids, channel.ID())
	}
	slices.Sort(ids)
	return ids
}

// A guild's channels are exactly those cached for it: added, updated, rejected by the policy, removed one by one
// or with their guild.
func TestChannelsForGuild(t *testing.T) {
	caches := New(WithCaches(FlagChannels), WithChannelCachePolicy(func(channel discord.GuildChannel) bool {
		return channel.Name() != "rejected"
	}))

	for _, id := range []snowflake.ID{10, 11, 12} {
		caches.AddChannel(textChannel(t, id, 1))
	}
	caches.AddChannel(textChannel(t, 20, 2))
	caches.AddChannel(textChannel(t, 10, 1)) // an update
	caches.AddChannel(guildChannel(t, `{"id":"13","type":0,"guild_id":"1","name":"rejected"}`))
	caches.AddChannel(guildChannel(t, `{"id":"12","type":0,"guild_id":"1","name":"rejected"}`)) // drops the cached one
	caches.RemoveChannel(11)

	if got, want := channelIDs(caches, 1), []snowflake.ID{10}; !slices.Equal(got, want) {
		t.Errorf("guild 1: got %v, want %v", got, want)
	}

	caches.RemoveChannelsByGuildID(1)
	if got := channelIDs(caches, 1); len(got) != 0 {
		t.Errorf("guild 1 removed: got %v, want none", got)
	}
	if _, ok := caches.Channel(10); ok {
		t.Error("guild 1 removed: channel 10 still cached")
	}
	if got, want := channelIDs(caches, 2), []snowflake.ID{20}; !slices.Equal(got, want) {
		t.Errorf("guild 1 removed, guild 2: got %v, want %v", got, want)
	}
}

// The loop body may change the cache: the iterator holds no lock while it yields.
func TestChannelsForGuildYieldsUnlocked(t *testing.T) {
	caches := New(WithCaches(FlagChannels))
	caches.AddChannel(textChannel(t, 20, 2))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for channel := range caches.ChannelsForGuild(2) {
			caches.RemoveChannel(channel.ID())
			caches.AddChannel(textChannel(t, 21, 2))
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlocked: changing the cache inside the loop")
	}

	if got, want := channelIDs(caches, 2), []snowflake.ID{21}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func BenchmarkChannelsForGuild(b *testing.B) {
	caches := New(WithCaches(FlagChannels))
	for guildID := snowflake.ID(1); guildID <= 1000; guildID++ {
		for index := range snowflake.ID(30) {
			caches.AddChannel(textChannel(b, guildID<<16|index, guildID))
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		for range caches.ChannelsForGuild(500) {
		}
	}
}

// An update, as every message's last_message_id makes one
func BenchmarkAddChannel(b *testing.B) {
	caches := New(WithCaches(FlagChannels))
	channel := textChannel(b, 10, 1)
	caches.AddChannel(channel)

	b.ReportAllocs()
	for b.Loop() {
		caches.AddChannel(channel)
	}
}
