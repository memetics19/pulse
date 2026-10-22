package diagnostics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const failedUnitsOutput = `jellyfin.service   loaded failed failed Jellyfin Media Server
zfs-mount.service  loaded failed failed Mount ZFS filesystems
`

func TestCollectSystemd_ParsesFailedUnits(t *testing.T) {
	runner := &fakeRunner{output: failedUnitsOutput}

	report, err := CollectSystemd(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, report.FailedUnits, 2)
	assert.Equal(t, "jellyfin.service", report.FailedUnits[0].Unit)
	assert.Equal(t, "Jellyfin Media Server", report.FailedUnits[0].Description)
}

func TestCollectSystemd_EmptyWhenNothingFailed(t *testing.T) {
	runner := &fakeRunner{output: "\n"}

	report, err := CollectSystemd(context.Background(), runner)

	require.NoError(t, err)
	assert.Empty(t, report.FailedUnits)
}
