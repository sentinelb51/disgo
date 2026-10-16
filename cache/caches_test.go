package cache

import (
	"fmt"
	"testing"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgo/discord"
)

func guildChannel(t *testing.T, raw string) discord.GuildChannel {
	t.Helper()
	var v discord.UnmarshalChannel
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v.Channel.(discord.GuildChannel)
}

// Discord applies the member's own overwrite after all role overwrites, so it wins over them.
func TestMemberPermissionsInChannelMemberOverwriteWins(t *testing.T) {
	caches := New(WithCaches(FlagGuilds, FlagRoles))
	caches.AddGuild(discord.CacheGuild{Guild: discord.Guild{ID: 1, OwnerID: 99}})
	caches.AddRole(discord.Role{ID: 1, GuildID: 1, Permissions: discord.PermissionViewChannel})
	caches.AddRole(discord.Role{ID: 2, GuildID: 1})

	// role 2 allows SEND_MESSAGES (2048), member 5 is denied it
	deniedMember := guildChannel(t, `{"id":"20","type":0,"guild_id":"1","permission_overwrites":[
		{"id":"2","type":0,"allow":"2048","deny":"0"},
		{"id":"5","type":1,"allow":"0","deny":"2048"}]}`)
	// role 2 denies SEND_MESSAGES, member 5 is allowed it
	allowedMember := guildChannel(t, `{"id":"21","type":0,"guild_id":"1","permission_overwrites":[
		{"id":"2","type":0,"allow":"0","deny":"2048"},
		{"id":"5","type":1,"allow":"2048","deny":"0"}]}`)

	member := discord.Member{GuildID: 1, User: discord.User{ID: 5}, RoleIDs: []snowflake.ID{2}}

	if perms := caches.MemberPermissionsInChannel(deniedMember, member); perms.Has(discord.PermissionSendMessages) {
		t.Errorf("member overwrite denies SEND_MESSAGES, role overwrite allows it: got %s, want it denied", perms)
	}
	if perms := caches.MemberPermissionsInChannel(allowedMember, member); perms.Missing(discord.PermissionSendMessages) {
		t.Errorf("member overwrite allows SEND_MESSAGES, role overwrite denies it: got %s, want it allowed", perms)
	}
}

func TestMemberPermissionsEveryoneAdministrator(t *testing.T) {
	caches := New(WithCaches(FlagGuilds, FlagRoles))
	caches.AddGuild(discord.CacheGuild{Guild: discord.Guild{ID: 1, OwnerID: 99}})
	caches.AddRole(discord.Role{ID: 1, GuildID: 1, Permissions: discord.PermissionAdministrator})

	member := discord.Member{GuildID: 1, User: discord.User{ID: 5}}
	if perms := caches.MemberPermissions(member); perms != discord.PermissionsAll {
		t.Errorf("@everyone has ADMINISTRATOR: got %s, want all permissions", perms)
	}
}

func BenchmarkMemberPermissions(b *testing.B) {
	for _, guildRoles := range []int{5, 50, 250} {
		b.Run(fmt.Sprintf("roles=%d", guildRoles), func(b *testing.B) {
			caches := New(WithCaches(FlagGuilds, FlagRoles))
			caches.AddGuild(discord.CacheGuild{Guild: discord.Guild{ID: 1, OwnerID: 99}})
			caches.AddRole(discord.Role{ID: 1, GuildID: 1, Permissions: discord.PermissionViewChannel})
			member := discord.Member{GuildID: 1, User: discord.User{ID: 5}}
			for i := range guildRoles {
				id := snowflake.ID(100 + i)
				caches.AddRole(discord.Role{ID: id, GuildID: 1, Permissions: discord.PermissionSendMessages})
				if i%(guildRoles/3) == 0 {
					member.RoleIDs = append(member.RoleIDs, id)
				}
			}

			b.ReportAllocs()
			for b.Loop() {
				caches.MemberPermissions(member)
			}
		})
	}
}

// An update that the policy rejects must not leave the previous version cached.
func TestPutPolicyRejectedUpdateRemoves(t *testing.T) {
	textOnly := func(channel discord.GuildChannel) bool { return channel.Type() == discord.ChannelTypeGuildText }
	caches := New(WithCaches(FlagMembers, FlagChannels), WithMemberCachePolicy(PolicyMembersPending), WithChannelCachePolicy(textOnly))

	caches.AddMember(discord.Member{GuildID: 1, User: discord.User{ID: 5}, Pending: true})
	caches.AddMember(discord.Member{GuildID: 1, User: discord.User{ID: 5}, Pending: false})
	if member, ok := caches.Member(1, 5); ok {
		t.Errorf("member that completed screening still cached as %+v", member)
	}

	caches.AddChannel(guildChannel(t, `{"id":"20","type":0,"guild_id":"1"}`))
	caches.AddChannel(guildChannel(t, `{"id":"20","type":5,"guild_id":"1"}`))
	if channel, ok := caches.Channel(20); ok {
		t.Errorf("channel converted to a type the policy excludes still cached as type %d", channel.Type())
	}
}
