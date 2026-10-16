package bot

import "testing"

func TestMemberChunkingFilterExcludeGuildIDs(t *testing.T) {
	filter := MemberChunkingFilterExcludeGuildIDs(1)

	if filter(1) {
		t.Error("excluded guild 1 is chunked")
	}
	if !filter(2) {
		t.Error("guild 2 is not chunked")
	}
}
