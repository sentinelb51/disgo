package gateway_test

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/bot/handlers"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

// BenchmarkMessageCreate times a MESSAGE_CREATE from its frame to a listener, as the read loop runs it: decoded as
// the transport decodes it, then handled by a client that caches servers, channels and roles, but no messages,
// holding the server and channel the message is in
func BenchmarkMessageCreate(b *testing.B) {

	client := &bot.Client{Caches: cache.New(cache.WithCaches(cache.FlagGuilds | cache.FlagChannels | cache.FlagRoles))}
	client.EventManager = bot.NewEventManager(client, bot.WithListeners(bot.NewListenerFunc(func(bot.Event) {})))
	handler := handlers.GetGatewayHandlers()[gateway.EventTypeMessageCreate]

	guildID := snowflake.ID(1102938475610293840)
	client.Caches.AddGuild(discord.CacheGuild{ID: guildID, Name: "guild"})

	var channel discord.UnmarshalChannel
	if err := jsonv2.Unmarshal([]byte(`{"id":"1102938475610293845","type":0,"name":"general","permission_overwrites":[{"id":"1102938475610293840","type":0,"allow":"0","deny":"2048"}]}`), &channel); err != nil {
		b.Fatal(err)
	}
	client.Caches.AddChannel(discord.ApplyGuildIDToChannel(channel.Channel.(discord.GuildChannel), guildID))

	var frame []byte
	for _, fixture := range fixtures(b) {
		if fixture.name == "message_create" {
			frame = fixture.frame
		}
	}

	reader := bytes.NewReader(nil)
	b.SetBytes(int64(len(frame)))
	b.ReportAllocs()
	for b.Loop() {
		reader.Reset(frame)
		reused.Reset(reader, streamOptions)

		var message gateway.Message
		if err := gateway.DecodeFrame(reused, &message, false); err != nil {
			b.Fatal(err)
		}

		handler.HandleGatewayEvent(client, message.S, 0, message.D.(gateway.EventMessageCreate))
	}
}

// BenchmarkGuildCreate times a GUILD_CREATE from its frame into the cache of a bot keeping servers, channels, roles,
// its own member, emojis and stickers, as one connecting to Discord handles each of its servers
func BenchmarkGuildCreate(b *testing.B) {

	client := &bot.Client{Caches: cache.New(
		cache.WithCaches(cache.FlagGuilds|cache.FlagChannels|cache.FlagRoles|cache.FlagMembers|cache.FlagEmojis|cache.FlagStickers),
		cache.WithMemberCachePolicy(func(member discord.Member) bool { return member.User.ID == 912345678901234567 }),
	)}
	client.EventManager = bot.NewEventManager(client, bot.WithListeners(bot.NewListenerFunc(func(bot.Event) {})))
	client.MemberChunkingManager = bot.NewMemberChunkingManager(client, nil, nil)
	handler := handlers.GetGatewayHandlers()[gateway.EventTypeGuildCreate]

	var frame []byte
	for _, fixture := range fixtures(b) {
		if fixture.name == "guild_create" {
			frame = fixture.frame
		}
	}

	reader := bytes.NewReader(nil)
	b.SetBytes(int64(len(frame)))
	b.ReportAllocs()
	for b.Loop() {
		reader.Reset(frame)
		reused.Reset(reader, streamOptions)

		var message gateway.Message
		if err := gateway.DecodeFrame(reused, &message, false); err != nil {
			b.Fatal(err)
		}

		handler.HandleGatewayEvent(client, message.S, 0, message.D.(gateway.EventGuildCreate))
	}
}
