package discord

import (
	"testing"
)

// Bot messages as MESSAGE_CREATE carries them: one with an embed and buttons, one built of components v2
var benchmarkMessages = []struct {
	name string
	data []byte
}{
	{"embed", []byte(`{"type":0,"tts":false,"timestamp":"2026-09-30T22:14:05.512000+00:00","pinned":false,"mentions":[],"mention_roles":[],"mention_everyone":false,"id":"1422876543210988000","flags":0,"embeds":[{"type":"rich","title":"Verification required","description":"Hello <@402938475610293847>, please verify your account by clicking the button below. This keeps the server safe from raids and alts.","color":3447003,"timestamp":"2026-09-30T22:14:05.000000+00:00","fields":[{"name":"Account age","value":"3 years","inline":true},{"name":"Joined","value":"<t:1727734445:R>","inline":true},{"name":"Risk","value":"low","inline":false}],"footer":{"text":"VOID","icon_url":"https://cdn.discordapp.com/avatars/1/abc.png","proxy_icon_url":"https://images-ext-1.discordapp.net/external/x/https/cdn.discordapp.com/avatars/1/abc.png"},"thumbnail":{"url":"https://cdn.discordapp.com/avatars/402938475610293847/1d2c3b4a59687f6e5d4c3b2a19087f6e.png","proxy_url":"https://images-ext-1.discordapp.net/external/y/https/cdn.discordapp.com/avatars/402938475610293847/1d2c3b4a59687f6e5d4c3b2a19087f6e.png","width":128,"height":128,"flags":0},"content_scan_version":2}],"edited_timestamp":null,"content":"","components":[{"type":1,"id":1,"components":[{"type":2,"id":2,"style":1,"label":"Verify","custom_id":"verify:402938475610293847"},{"type":2,"id":3,"style":5,"label":"Help","url":"https://void.enterprises/help"}]}],"channel_type":0,"channel_id":"1102938475610293845","author":{"username":"VOID","public_flags":65536,"primary_guild":null,"id":"912345678901234567","global_name":null,"display_name_styles":null,"discriminator":"1234","collectibles":null,"clan":null,"bot":true,"avatar_decoration_data":null,"avatar":"abcdef0123456789abcdef0123456789"},"attachments":[],"guild_id":"1102938475610293840","member":{"roles":["1102938475610293841"],"premium_since":null,"pending":false,"nick":null,"mute":false,"joined_at":"2026-09-30T20:11:02.114000+00:00","flags":0,"deaf":false,"communication_disabled_until":null,"banner":null,"avatar":null}}`)},
	{"components_v2", []byte(`{"type":0,"tts":false,"timestamp":"2026-09-30T22:14:05.512000+00:00","pinned":false,"mentions":[],"mention_roles":[],"mention_everyone":false,"id":"1422876543210988000","flags":32768,"embeds":[],"edited_timestamp":null,"content":"","components":[{"type":17,"id":1,"accent_color":3447003,"components":[{"type":10,"id":2,"content":"## Verification required"},{"type":9,"id":3,"components":[{"type":10,"id":4,"content":"Hello <@402938475610293847>, please verify your account by clicking the button below."}],"accessory":{"type":11,"id":5,"media":{"url":"https://cdn.discordapp.com/avatars/402938475610293847/1d2c3b4a59687f6e5d4c3b2a19087f6e.png","proxy_url":"https://media.discordapp.net/avatars/402938475610293847/1d2c3b4a59687f6e5d4c3b2a19087f6e.png","width":128,"height":128,"placeholder":"Q/eFBQBId3eAeIiId3d4h3d3eA==","placeholder_version":1,"content_type":"image/png","loading_state":2,"flags":0}}},{"type":14,"id":6,"divider":true,"spacing":1},{"type":10,"id":7,"content":"-# Account age 3 years, joined <t:1727734445:R>"},{"type":1,"id":8,"components":[{"type":2,"id":9,"style":1,"label":"Verify","custom_id":"verify:402938475610293847"},{"type":2,"id":10,"style":5,"label":"Help","url":"https://void.enterprises/help"}]}]}],"channel_type":0,"channel_id":"1102938475610293845","author":{"username":"VOID","public_flags":65536,"primary_guild":null,"id":"912345678901234567","global_name":null,"display_name_styles":null,"discriminator":"1234","collectibles":null,"clan":null,"bot":true,"avatar_decoration_data":null,"avatar":"abcdef0123456789abcdef0123456789"},"attachments":[],"guild_id":"1102938475610293840","member":{"roles":["1102938475610293841"],"premium_since":null,"pending":false,"nick":null,"mute":false,"joined_at":"2026-09-30T20:11:02.114000+00:00","flags":0,"deaf":false,"communication_disabled_until":null,"banner":null,"avatar":null}}`)},
}

func BenchmarkMessage_UnmarshalJSON(b *testing.B) {
	for _, message := range benchmarkMessages {
		b.Run(message.name, func(b *testing.B) {
			b.SetBytes(int64(len(message.data)))
			b.ReportAllocs()
			for b.Loop() {
				var m Message
				if err := m.UnmarshalJSON(message.data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
