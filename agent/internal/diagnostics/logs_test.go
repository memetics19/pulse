package diagnostics

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// funcRunner dispatches on the full command, so a test can answer `docker ps`
// and `docker logs` differently.
type funcRunner func(name string, args []string) ([]byte, error)

func (f funcRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return f(name, args)
}

// An OOM kill says a service died; the log says why. Capturing the journal for
// units already known to be failed needs no extra input from the server.
func TestCollectSystemd_CapturesJournalForFailedUnits(t *testing.T) {
	var journalArgs []string
	runner := funcRunner(func(name string, args []string) ([]byte, error) {
		switch name {
		case "systemctl":
			return []byte(failedUnitsOutput), nil
		case "journalctl":
			journalArgs = args
			return []byte("Aug 16 03:14:02 host jellyfin[912]: Fatal: out of memory\n"), nil
		}
		return nil, nil
	})

	report, err := CollectSystemd(context.Background(), runner)

	require.NoError(t, err)
	require.Contains(t, report.Logs, "jellyfin.service")
	assert.Contains(t, report.Logs["jellyfin.service"], "out of memory")
	assert.Contains(t, journalArgs, "--no-pager", "journalctl must not block on a pager")
}

func TestCollectSystemd_NoLogsWhenNothingFailed(t *testing.T) {
	runner := funcRunner(func(name string, _ []string) ([]byte, error) {
		if name == "journalctl" {
			t.Fatal("journalctl must not run when no unit has failed")
		}
		return []byte("\n"), nil
	})

	report, err := CollectSystemd(context.Background(), runner)

	require.NoError(t, err)
	assert.Empty(t, report.Logs)
}

func TestCollectDocker_CapturesLogsForNonRunningContainers(t *testing.T) {
	runner := funcRunner(func(name string, args []string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "logs" {
			return []byte("Killed\n"), nil
		}
		return []byte(dockerPSOutput), nil
	})

	report, err := CollectDocker(context.Background(), runner)

	require.NoError(t, err)
	require.Contains(t, report.Logs, "jellyfin")
	assert.Contains(t, report.Logs["jellyfin"], "Killed")
	assert.NotContains(t, report.Logs, "caddy", "a healthy container needs no log dump")
}

// The whole bundle is POSTed under the server's 1 MiB body cap, so a single
// runaway log must not make the bundle unsendable.
func TestCollectDocker_TruncatesOversizedLogs(t *testing.T) {
	huge := strings.Repeat("x", 200_000)
	runner := funcRunner(func(name string, args []string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "logs" {
			return []byte(huge), nil
		}
		return []byte(dockerPSOutput), nil
	})

	report, err := CollectDocker(context.Background(), runner)

	require.NoError(t, err)
	assert.LessOrEqual(t, len(report.Logs["jellyfin"]), maxLogBytes+len(logTruncationNotice))
	assert.Contains(t, report.Logs["jellyfin"], "truncated")
}
