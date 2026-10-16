package discord

import (
	"reflect"
	"testing"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"
)

func TestRoleTag_UnmarshalJSON(t *testing.T) {
	botID := snowflake.ID(1)
	listingID := snowflake.ID(2)

	tests := []struct {
		name string
		data string
		want RoleTag
	}{
		{"none", `{}`, RoleTag{}},
		{"booster", `{"premium_subscriber":null}`, RoleTag{PremiumSubscriber: true}},
		{"purchasable", `{"subscription_listing_id":"2","available_for_purchase":null}`, RoleTag{SubscriptionListingID: &listingID, AvailableForPurchase: true}},
		{"linked", `{"guild_connections":null}`, RoleTag{GuildConnections: true}},
		{"bot", `{"bot_id":"1"}`, RoleTag{BotID: &botID}},
		{"marshaled", `{"premium_subscriber":true,"available_for_purchase":false,"guild_connections":false}`, RoleTag{PremiumSubscriber: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var role Role
			if err := json.Unmarshal([]byte(`{"id":"3","tags":`+tt.data+`}`), &role); err != nil {
				t.Fatal(err)
			}
			if role.Tags == nil {
				t.Fatal("tags not decoded")
			}
			if !reflect.DeepEqual(*role.Tags, tt.want) {
				t.Errorf("got %+v, want %+v", *role.Tags, tt.want)
			}
		})
	}
}
