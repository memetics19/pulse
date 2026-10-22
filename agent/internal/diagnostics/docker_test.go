package diagnostics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docker ps --format '{{json .}}' emits one JSON object per line.
const dockerPSOutput = `{"ID":"a1b2c3","Image":"jellyfin/jellyfin","Names":"jellyfin","State":"exited","Status":"Exited (137) 5 minutes ago"}
{"ID":"d4e5f6","Image":"caddy:2","Names":"caddy","State":"running","Status":"Up 3 days"}
`

func TestCollectDocker_ParsesContainers(t *testing.T) {
	runner := &fakeRunner{output: dockerPSOutput}

	report, err := CollectDocker(context.Background(), runner)

	require.NoError(t, err)
	require.Len(t, report.Containers, 2)
	assert.Equal(t, "jellyfin", report.Containers[0].Name)
	assert.Equal(t, "exited", report.Containers[0].State)
}

// Exit 137 is SIGKILL, which on a container almost always means the OOM killer.
// Surfacing it directly saves cross-referencing dmesg by hand.
func TestCollectDocker_FlagsNonRunningContainers(t *testing.T) {
	runner := &fakeRunner{output: dockerPSOutput}

	report, err := CollectDocker(context.Background(), runner)

	require.NoError(t, err)
	assert.Equal(t, []string{"jellyfin"}, report.NotRunning)
}

// Skipping malformed rows silently can hide the very container being
// diagnosed. If nothing at all parses, the section is not healthy-and-empty.
func TestCollectDocker_FailsWhenNoRowParses(t *testing.T) {
	runner := &fakeRunner{output: "not json\nalso not json\n"}

	_, err := CollectDocker(context.Background(), runner)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no containers parsed")
}
