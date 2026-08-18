package diagnostics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real dmesg output from a kernel OOM kill, trimmed to the relevant lines.
const dmesgWithOOM = `[Sat Aug 16 03:13:58 2026] jellyfin invoked oom-killer: gfp_mask=0x140cca(GFP_HIGHUSER_MOVABLE|__GFP_COMP), order=0, oom_score_adj=0
[Sat Aug 16 03:14:02 2026] oom-kill:constraint=CONSTRAINT_NONE,nodemask=(null),cpuset=/,mems_allowed=0,global_oom,task_memcg=/system.slice/docker-abc.scope,task=jellyfin,pid=1234,uid=0
[Sat Aug 16 03:14:02 2026] Out of memory: Killed process 1234 (jellyfin) total-vm:8216044kB, anon-rss:7104928kB, file-rss:0kB, shmem-rss:0kB, UID:0 pgtables:15000kB oom_score_adj:0
`

func TestParseOOMKills_ExtractsProcessAndPID(t *testing.T) {
	kills := parseOOMKills(dmesgWithOOM)

	require.Len(t, kills, 1)
	assert.Equal(t, "jellyfin", kills[0].Process)
	assert.Equal(t, 1234, kills[0].PID)
	assert.Equal(t, int64(7104928), kills[0].AnonRSSKb)
}

func TestParseOOMKills_NoneWhenDmesgIsClean(t *testing.T) {
	clean := "[Sat Aug 16 03:14:02 2026] eth0: link up\n"

	assert.Empty(t, parseOOMKills(clean))
}
