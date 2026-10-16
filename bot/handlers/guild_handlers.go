package handlers

import (
	"context"
	"log/slog"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

func gatewayHandlerGuildCreate(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventGuildCreate) {
	if event.Unavailable {
		// during an outage the payload is only an unavailable guild object
		gatewayHandlerGuildDelete(client, sequenceNumber, shardID, gateway.EventGuildDelete{UnavailableGuild: discord.UnavailableGuild{ID: event.ID, Unavailable: true}})
		return
	}

	wasUnready := client.Caches.IsGuildUnready(event.ID)
	wasUnavailable := client.Caches.IsGuildUnavailable(event.ID)

	client.Caches.AddGuild(event.CacheGuild)

	for _, channel := range event.Channels {
		channel = discord.ApplyGuildIDToChannel(channel, event.ID) // populate unset field
		client.Caches.AddChannel(channel)
	}

	for _, thread := range event.Threads {
		thread = discord.ApplyGuildIDToThread(thread, event.ID) // populate unset field
		client.Caches.AddChannel(thread)
	}

	for _, role := range event.Roles {
		role.GuildID = event.ID // populate unset field
		client.Caches.AddRole(role)
	}

	for _, member := range event.Members {
		member.GuildID = event.ID // populate unset field
		client.Caches.AddMember(member)
	}

	for _, voiceState := range event.VoiceStates {
		voiceState.GuildID = event.ID // populate unset field
		client.Caches.AddVoiceState(voiceState)
	}

	for _, emoji := range event.Emojis {
		emoji.GuildID = event.ID // populate unset field
		client.Caches.AddEmoji(emoji)
	}

	for _, sticker := range event.Stickers {
		sticker.GuildID = &event.ID // populate unset field
		client.Caches.AddSticker(sticker)
	}

	for _, stageInstance := range event.StageInstances {
		client.Caches.AddStageInstance(stageInstance)
	}

	for _, guildScheduledEvent := range event.GuildScheduledEvents {
		client.Caches.AddGuildScheduledEvent(guildScheduledEvent)
	}

	for _, soundboardSound := range event.SoundboardSounds {
		client.Caches.AddGuildSoundboardSound(soundboardSound)
	}

	for _, presence := range event.Presences {
		presence.GuildID = event.ID // populate unset field
		client.Caches.AddPresence(presence)
	}

	genericGuildEvent := &events.GenericGuild{
		GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
		GuildID:      event.ID,
	}

	if wasUnready {
		client.Caches.SetGuildUnready(event.ID, false)
		client.Caches.SetGuildUnavailable(event.ID, false)
		client.EventManager.DispatchEvent(&events.GuildReady{
			GenericGuild: genericGuildEvent,
			Guild:        event.CacheGuild,
		})
		if client.Caches.UnreadyGuildsLen() == 0 {
			client.EventManager.DispatchEvent(&events.GuildsReady{
				GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
			})
		}
		if client.MemberChunkingManager.MemberChunkingFilter()(event.ID) {
			go func() {
				if _, err := client.MemberChunkingManager.RequestMembersWithQuery(context.Background(), event.ID, "", 0); err != nil {
					client.Logger.Error("failed to chunk guild on guild_create", slog.Any("err", err))
				}
			}()
		}

		return
	}
	if wasUnavailable {
		client.Caches.SetGuildUnavailable(event.ID, false)
		client.EventManager.DispatchEvent(&events.GuildAvailable{
			GenericGuild: genericGuildEvent,
			Guild:        event.CacheGuild,
		})
	} else {
		client.EventManager.DispatchEvent(&events.GuildJoin{
			GenericGuild: genericGuildEvent,
			Guild:        event.CacheGuild,
		})
	}
}

func gatewayHandlerGuildUpdate(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventGuildUpdate) {
	oldGuild, _ := client.Caches.Guild(event.ID)
	client.Caches.AddGuild(discord.CacheGuild{
		Guild:       event.Guild,
		JoinedAt:    oldGuild.JoinedAt,
		Large:       oldGuild.Large,
		MemberCount: oldGuild.MemberCount,
	})

	client.EventManager.DispatchEvent(&events.GuildUpdate{
		GenericGuild: &events.GenericGuild{
			GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
			GuildID:      event.ID,
		},
		Guild:    event.Guild,
		OldGuild: oldGuild,
	})
}

func gatewayHandlerGuildDelete(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventGuildDelete) {
	// without unavailable the bot left the guild: it is no longer unavailable or awaited
	wasUnready := !event.Unavailable && client.Caches.IsGuildUnready(event.ID)
	client.Caches.SetGuildUnavailable(event.ID, event.Unavailable)
	if wasUnready {
		client.Caches.SetGuildUnready(event.ID, false)
	}

	guild, _ := client.Caches.RemoveGuild(event.ID)
	removeGuildEntities(client, event.ID)

	genericGuildEvent := &events.GenericGuild{
		GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
		GuildID:      event.ID,
	}

	if event.Unavailable {
		client.EventManager.DispatchEvent(&events.GuildUnavailable{
			GenericGuild: genericGuildEvent,
			Guild:        guild,
		})
	} else {
		client.EventManager.DispatchEvent(&events.GuildLeave{
			GenericGuild: genericGuildEvent,
			Guild:        guild,
		})
		if wasUnready && client.Caches.UnreadyGuildsLen() == 0 {
			client.EventManager.DispatchEvent(&events.GuildsReady{
				GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
			})
		}
	}
}

// removeGuildEntities removes everything cached for the given guilds except the guilds themselves. It walks the
// channel cache once, however many guilds it is given.
func removeGuildEntities(client *bot.Client, guildIDs ...snowflake.ID) {
	if len(guildIDs) == 0 {
		return
	}
	removed := make(map[snowflake.ID]struct{}, len(guildIDs))
	for _, guildID := range guildIDs {
		removed[guildID] = struct{}{}
		client.Caches.RemoveVoiceStatesByGuildID(guildID)
		client.Caches.RemovePresencesByGuildID(guildID)
		client.Caches.RemoveEmojisByGuildID(guildID)
		client.Caches.RemoveStickersByGuildID(guildID)
		client.Caches.RemoveRolesByGuildID(guildID)
		client.Caches.RemoveMembersByGuildID(guildID)
		client.Caches.RemoveStageInstancesByGuildID(guildID)
		client.Caches.RemoveGuildScheduledEventsByGuildID(guildID)
		client.Caches.RemoveGuildSoundboardSoundsByGuildID(guildID)
		client.Caches.RemoveMessagesByGuildID(guildID)
	}

	// TODO: figure out a better way to remove thread members from cache via guild id without requiring cached GuildThreads
	var channels []discord.GuildChannel
	for channel := range client.Caches.Channels() {
		if _, ok := removed[channel.GuildID()]; ok {
			channels = append(channels, channel)
		}
	}
	for _, channel := range channels {
		if _, ok := channel.(discord.GuildThread); ok {
			client.Caches.RemoveThreadMembersByThreadID(channel.ID())
		}
		client.Caches.RemoveChannel(channel.ID())
	}
}

func gatewayHandlerGuildAuditLogEntryCreate(client *bot.Client, sequenceNumber int, shardID int, event gateway.EventGuildAuditLogEntryCreate) {
	genericGuildEvent := &events.GenericGuild{
		GenericEvent: events.NewGenericEvent(client, sequenceNumber, shardID),
		GuildID:      event.GuildID,
	}

	client.EventManager.DispatchEvent(&events.GuildAuditLogEntryCreate{
		GenericGuild:  genericGuildEvent,
		AuditLogEntry: event.AuditLogEntry,
	})
}
