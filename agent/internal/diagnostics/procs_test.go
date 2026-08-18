package diagnostics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const psOutput = `  PID %CPU %MEM COMMAND
 4821 187.3  4.2 ffmpeg
  912   2.1  8.0 jellyfin
    1   0.0  0.1 systemd
`

func TestCollectProcesses_ParsesTopByCPU(t *testing.T) {
	runner := &fakeRunner{output: psOutput}

	report, err := CollectProcesses(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, report.Top, 3)

	assert.Equal(t, 4821, report.Top[0].PID)
	assert.Equal(t, "ffmpeg", report.Top[0].Command)
	assert.InDelta(t, 187.3, report.Top[0].CPUPercent, 0.01)
	assert.InDelta(t, 4.2, report.Top[0].MemPercent, 0.01)
}
