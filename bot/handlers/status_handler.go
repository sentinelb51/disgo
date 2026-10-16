package handlers

import (
	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/sharding"
)

func gatewayHandlerRaw(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventRaw) {
	client.EventManager.DispatchEvent(&events.Raw{
		GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
		EventRaw:     event,
	})
}

func gatewayHandlerHeartbeatAck(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventHeartbeatAck) {
	client.EventManager.DispatchEvent(&events.HeartbeatAck{
		GenericEvent:      events.NewGenericEvent(client, sequenceNumber, shardID),
		EventHeartbeatAck: event,
	})
}

func gatewayHandlerRateLimited(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventRateLimited) {
	client.EventManager.DispatchEvent(&events.GatewayRateLimited{
		GenericEvent:     events.NewGenericEvent(client, sequenceNumber, shardID),
		EventRateLimited: event,
	})
}

func gatewayHandlerReady(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventReady) {
	client.Caches.SetSelfUser(event.User)

	// A new session sends a GUILD_CREATE for each guild of the shard and nothing for what changed without a session
	// (guilds left, channels or roles deleted): drop what earlier sessions cached for the shard's guilds.
	shardCount := max(event.Shard[1], 1)
	inShard := func(guildID snowflake.ID) bool {
		return sharding.ShardIDByGuild(guildID, shardCount) == event.Shard[0]
	}
	var guildIDs []snowflake.ID
	for guild := range client.Caches.Guilds() {
		if inShard(guild.ID) {
			guildIDs = append(guildIDs, guild.ID)
		}
	}
	for _, guildID := range guildIDs {
		client.Caches.RemoveGuild(guildID)
	}
	removeGuildEntities(client, guildIDs...)
	for _, guildID := range client.Caches.UnreadyGuildIDs() {
		if inShard(guildID) {
			client.Caches.SetGuildUnready(guildID, false)
		}
	}
	for _, guildID := range client.Caches.UnavailableGuildIDs() {
		if inShard(guildID) {
			client.Caches.SetGuildUnavailable(guildID, false)
		}
	}

	for _, guild := range event.Guilds {
		client.Caches.SetGuildUnready(guild.ID, true)
	}

	client.EventManager.DispatchEvent(&events.Ready{
		GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
		EventReady:   event,
	})
}

func gatewayHandlerResumed(client *bot.Client, sequenceNumber int, shardID int, _ gateway.EventData) {
	client.EventManager.DispatchEvent(&events.Resumed{
		GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
	})
}
