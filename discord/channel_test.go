package discord

import (
	"reflect"
	"testing"
)

// benchmarkChannel is a text channel as CHANNEL_UPDATE and GUILD_CREATE carry it, with role and member overwrites
var benchmarkChannel = []byte(`{"version":1727734445000,"type":0,"topic":"Talk about anything. Read #rules first.","rate_limit_per_user":5,"position":3,"permission_overwrites":[{"type":0,"id":"1102938475610293840","deny":"1024","allow":"0"},{"type":0,"id":"1102938475610293847","deny":"0","allow":"3072"},{"type":0,"id":"1102938475610293850","deny":"0","allow":"8192"},{"type":1,"id":"912345678901234567","deny":"1024","allow":"0"},{"type":0,"id":"1211234567890123456","deny":"0","allow":"1024"}],"parent_id":"1102938475610293841","nsfw":false,"name":"general","last_message_id":"1422876543210987600","id":"1102938475610293845","icon_emoji":{"name":"👋","id":null},"guild_id":"1102938475610293840","flags":0,"theme_color":null}`)

func TestUnmarshalChannel_PermissionOverwrites(t *testing.T) {
	var channel UnmarshalChannel
	if err := channel.UnmarshalJSON(benchmarkChannel); err != nil {
		t.Fatal(err)
	}

	want := PermissionOverwrites{
		RolePermissionOverwrite{RoleID: 1102938475610293840, Deny: PermissionViewChannel},
		RolePermissionOverwrite{RoleID: 1102938475610293847, Allow: PermissionViewChannel | PermissionSendMessages},
		RolePermissionOverwrite{RoleID: 1102938475610293850, Allow: PermissionManageMessages},
		MemberPermissionOverwrite{UserID: 912345678901234567, Deny: PermissionViewChannel},
		RolePermissionOverwrite{RoleID: 1211234567890123456, Allow: PermissionViewChannel},
	}
	if got := channel.Channel.(GuildTextChannel).PermissionOverwrites(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func BenchmarkUnmarshalChannel(b *testing.B) {
	b.SetBytes(int64(len(benchmarkChannel)))
	b.ReportAllocs()
	for b.Loop() {
		var channel UnmarshalChannel
		if err := channel.UnmarshalJSON(benchmarkChannel); err != nil {
			b.Fatal(err)
		}
	}
}
