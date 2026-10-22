package diagnostics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const qmListOutput = `      VMID NAME                 STATUS     MEM(MB)    BOOTDISK(GB) PID
       101 nas                  running    8192              32.00 1234
       102 buildbox             stopped    2048              16.00 0
`

func TestCollectProxmox_ParsesVMList(t *testing.T) {
	runner := &fakeRunner{output: qmListOutput}

	report, err := CollectProxmox(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, report.VMs, 2)
	assert.Equal(t, 101, report.VMs[0].VMID)
	assert.Equal(t, "nas", report.VMs[0].Name)
	assert.Equal(t, "running", report.VMs[0].Status)
}

// Healing a hung VM from inside it is unreliable, so the host-side view of
// which VMs are not running is the actionable signal.
func TestCollectProxmox_FlagsStoppedVMs(t *testing.T) {
	runner := &fakeRunner{output: qmListOutput}

	report, err := CollectProxmox(context.Background(), runner)

	require.NoError(t, err)
	assert.Equal(t, []string{"buildbox"}, report.NotRunning)
}
