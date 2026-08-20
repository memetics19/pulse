package diagnostics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dfOutput = `Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/sda1        102687672  97553436         0     100% /
tmpfs             16440000       124  16439876       1% /dev/shm
/dev/sdb1       1922728840 961364420 863728412      53% /mnt/media
`

func TestCollectDisk_ParsesMountUsage(t *testing.T) {
	runner := &fakeRunner{output: dfOutput}

	report, err := CollectDisk(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, report.Mounts, 3)

	root := report.Mounts[0]
	assert.Equal(t, "/", root.Mount)
	assert.Equal(t, "/dev/sda1", root.Filesystem)
	assert.Equal(t, 100, root.CapacityPercent)
	assert.Equal(t, int64(0), root.AvailableKb)
}

// The disk-full death spiral is a named risk, so "which mount is full" has to
// be answerable directly from the bundle.
func TestCollectDisk_FlagsMountsAtCapacity(t *testing.T) {
	runner := &fakeRunner{output: dfOutput}

	report, err := CollectDisk(context.Background(), runner)

	require.NoError(t, err)
	assert.Equal(t, []string{"/"}, report.Full)
}

// Only filesystems that are 100% by design are suppressed. A full tmpfs is
// real memory-backed exhaustion and a full overlay is a container's writable
// layer filling up — both are actionable and must be flagged.
func TestCollectDisk_FlagsRealExhaustionOnTmpfsAndOverlay(t *testing.T) {
	const mixed = `Filesystem     1024-blocks      Used Available Capacity Mounted on
devfs                  394       394         0     100% /dev
/dev/loop0           65536     65536         0     100% /snap/core
tmpfs             16440000  16440000         0     100% /run
overlay          102687672  102687672        0     100% /var/lib/docker/overlay2/abc/merged
/dev/sda1        102687672  97553436         0     100% /
`
	runner := &fakeRunner{output: mixed}

	report, err := CollectDisk(context.Background(), runner)

	require.NoError(t, err)
	// Loop-backed mounts are flagged too: writable loop-mounted ext4/XFS is
	// common, and hiding a genuinely full filesystem is worse than an
	// occasional spurious flag on a read-only image.
	assert.Equal(t, []string{"/snap/core", "/run", "/var/lib/docker/overlay2/abc/merged", "/"}, report.Full)
	assert.Len(t, report.Mounts, 5, "all mounts stay listed for context")
}

// devfs has no backing store and reports 100% permanently; flagging it would
// be a false "disk full" on a healthy host.
func TestCollectDisk_IgnoresFilesystemsThatAreAlwaysFull(t *testing.T) {
	const pseudoOnly = `Filesystem     1024-blocks      Used Available Capacity Mounted on
devfs                  394       394         0     100% /dev
`
	runner := &fakeRunner{output: pseudoOnly}

	report, err := CollectDisk(context.Background(), runner)

	require.NoError(t, err)
	assert.Empty(t, report.Full)
}

// POSIX df -P reports 512-byte blocks, and this host does exactly that, so the
// unit has to be forced or available_kb is silently double the real value.
func TestCollectDisk_ForcesKilobyteUnits(t *testing.T) {
	runner := &fakeRunner{output: dfOutput}

	_, err := CollectDisk(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "df -Pk", runner.calls[0])
}
