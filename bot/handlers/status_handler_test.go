package handlers

import (
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

func TestRateLimitedDispatchesGatewayRateLimited(t *testing.T) {
	client := newTestClient(t, cache.FlagsNone)
	var got *events.GatewayRateLimited
	client.AddEventListeners(bot.NewListenerFunc(func(e *events.GatewayRateLimited) { got = e }))

	handler, ok := GetGatewayHandlers()[gateway.EventTypeRateLimited]
	if !ok {
		t.Fatal("no handler for RATE_LIMITED")
	}
	handler.HandleGatewayEvent(client, 0, 0, gateway.EventRateLimited{
		Opcode:     gateway.OpcodeRequestGuildMembers,
		RetryAfter: 1.5,
		Meta:       gateway.RateLimitedMetadataRequestGuildMembers{GuildID: 1, Nonce: "n"},
	})

	if got == nil {
		t.Fatal("GatewayRateLimited not dispatched")
	}
	if meta, ok := got.Meta.(gateway.RateLimitedMetadataRequestGuildMembers); !ok || meta.Nonce != "n" || got.RetryAfter != 1.5 {
		t.Errorf("GatewayRateLimited = %+v, want the event's fields", got.EventRateLimited)
	}
}
