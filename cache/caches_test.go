package cache

import (
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
