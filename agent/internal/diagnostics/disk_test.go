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

// Pseudo filesystems permanently report 100% capacity. Flagging them as full
// would produce a false "disk full" diagnosis — and, once remediation exists,
// trigger the wrong action against a healthy host.
func TestCollectDisk_IgnoresPseudoFilesystemsWhenFlaggingFull(t *testing.T) {
	const withPseudoFS = `Filesystem     1024-blocks      Used Available Capacity Mounted on
devfs                  394       394         0     100% /dev
tmpfs             16440000  16440000         0     100% /run/lock
/dev/sda1        102687672  97553436         0     100% /
`
	runner := &fakeRunner{output: withPseudoFS}

	report, err := CollectDisk(context.Background(), runner)

	require.NoError(t, err)
	assert.Equal(t, []string{"/"}, report.Full,
		"only real block devices should be reported full")
	assert.Len(t, report.Mounts, 3, "all mounts stay listed for context")
}
