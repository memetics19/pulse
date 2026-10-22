package diagnostics

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptedRunner replies per command name, so a test can make one collector
// fail while the rest succeed.
type scriptedRunner struct {
	output map[string]string
	fail   map[string]error
}

func (s *scriptedRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if err, ok := s.fail[name]; ok {
		return nil, err
	}
	return []byte(s.output[name]), nil
}

func healthyHost() *scriptedRunner {
	return &scriptedRunner{output: map[string]string{
		"dmesg":     dmesgWithOOM,
		"df":        dfOutput,
		"ps":        psOutput,
		"docker":    dockerPSOutput,
		"systemctl": failedUnitsOutput,
		"qm":        qmListOutput,
	}}
}

func TestCollect_PopulatesEverySection(t *testing.T) {
	bundle := Collect(context.Background(), healthyHost())

	for _, section := range []string{"kernel", "disk", "processes", "docker", "systemd", "proxmox"} {
		require.Contains(t, bundle.Sections, section)
		assert.Empty(t, bundle.Sections[section].Error, "section %q should have succeeded", section)
	}
	assert.False(t, bundle.CollectedAt.IsZero(), "bundle must be timestamped")
}

// Most hosts are not Proxmox nodes and many deny dmesg. Neither may cost us the
// sections that did work — that is the whole point of per-section degradation.
func TestCollect_SurvivesCollectorsThatCannotRun(t *testing.T) {
	host := healthyHost()
	host.fail = map[string]error{
		"qm":    errors.New("executable file not found in $PATH"),
		"dmesg": errors.New("operation not permitted"),
	}

	bundle := Collect(context.Background(), host)

	assert.Contains(t, bundle.Sections["proxmox"].Error, "not found")
	assert.Contains(t, bundle.Sections["kernel"].Error, "not permitted")
	assert.Empty(t, bundle.Sections["disk"].Error, "disk must survive its siblings failing")
	assert.NotNil(t, bundle.Sections["disk"].Data)
}
