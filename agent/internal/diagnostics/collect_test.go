package diagnostics

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner stands in for the OS. It records what was invoked and replays a
// canned result, so collector tests never depend on the host they run on.
type fakeRunner struct {
	calls  []string
	output string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(append([]string{name}, args...), " "))
	return []byte(f.output), f.err
}

func TestCollectKernel_ReportsOOMKills(t *testing.T) {
	runner := &fakeRunner{output: dmesgWithOOM}

	report, err := CollectKernel(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, report.OOMKills, 1)
	assert.Equal(t, "jellyfin", report.OOMKills[0].Process)
}

func TestCollectKernel_SurfacesRunnerFailure(t *testing.T) {
	runner := &fakeRunner{err: errors.New("dmesg: operation not permitted")}

	_, err := CollectKernel(context.Background(), runner)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation not permitted")
}
