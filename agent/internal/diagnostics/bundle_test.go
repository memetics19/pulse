package diagnostics

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundleAdd_RecordsDataWhenCollectorSucceeds(t *testing.T) {
	var b Bundle

	b.Add("kernel", KernelReport{OOMKills: []OOMKill{{Process: "jellyfin"}}}, nil)

	require.Contains(t, b.Sections, "kernel")
	assert.Empty(t, b.Sections["kernel"].Error)
	assert.NotNil(t, b.Sections["kernel"].Data)
}

// A host may deny dmesg, lack docker, or not be a Proxmox node. None of that
// should cost us the sections that did work.
func TestBundleAdd_FailingCollectorDegradesOnlyItsOwnSection(t *testing.T) {
	var b Bundle

	b.Add("kernel", KernelReport{}, errors.New("operation not permitted"))
	b.Add("disk", map[string]any{"/": 91.4}, nil)

	assert.Equal(t, "operation not permitted", b.Sections["kernel"].Error)
	assert.Nil(t, b.Sections["kernel"].Data, "failed section must not carry partial data")

	assert.Empty(t, b.Sections["disk"].Error)
	assert.NotNil(t, b.Sections["disk"].Data, "healthy section must survive a sibling failure")
}

// The bundle is stored as a JSON TEXT column, so it has to round-trip.
func TestBundle_RoundTripsThroughJSON(t *testing.T) {
	var b Bundle
	b.Add("kernel", KernelReport{OOMKills: []OOMKill{{Process: "jellyfin", PID: 1234}}}, nil)

	encoded, err := json.Marshal(b)
	require.NoError(t, err)

	var decoded Bundle
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Contains(t, string(encoded), "jellyfin")
	assert.Contains(t, decoded.Sections, "kernel")
}
