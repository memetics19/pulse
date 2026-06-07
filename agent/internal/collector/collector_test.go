package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnapshot_ReturnsNonNegativeValues exercises the live OS path.
// It cannot fail on a healthy machine; it verifies no error and sane bounds.
func TestSnapshot_ReturnsNonNegativeValues(t *testing.T) {
	c := New()
	m, err := c.Snapshot()
	require.NoError(t, err)

	assert.GreaterOrEqual(t, m.CpuPercent, 0.0, "CpuPercent must be >= 0")
	assert.LessOrEqual(t, m.CpuPercent, 100.0, "CpuPercent must be <= 100")

	assert.GreaterOrEqual(t, m.MemPercent, 0.0, "MemPercent must be >= 0")
	assert.LessOrEqual(t, m.MemPercent, 100.0, "MemPercent must be <= 100")

	assert.GreaterOrEqual(t, m.MemUsedMb, 0.0, "MemUsedMb must be >= 0")

	assert.GreaterOrEqual(t, m.DiskPercent, 0.0, "DiskPercent must be >= 0")
	assert.LessOrEqual(t, m.DiskPercent, 100.0, "DiskPercent must be <= 100")

	assert.GreaterOrEqual(t, m.DiskUsedGb, 0.0, "DiskUsedGb must be >= 0")

	// First call: deltas must be 0 (no prior sample)
	assert.Equal(t, int64(0), m.NetRxBytes, "first NetRxBytes must be 0")
	assert.Equal(t, int64(0), m.NetTxBytes, "first NetTxBytes must be 0")
}

// TestApplyNetDelta_FirstCall checks delta is 0 on the first invocation.
func TestApplyNetDelta_FirstCall(t *testing.T) {
	c := New()
	rx, tx := c.applyNetDelta(1000, 2000)
	assert.Equal(t, int64(0), rx, "first call rx delta must be 0")
	assert.Equal(t, int64(0), tx, "first call tx delta must be 0")
}

// TestApplyNetDelta_SecondCall checks delta is computed correctly.
func TestApplyNetDelta_SecondCall(t *testing.T) {
	c := New()
	c.applyNetDelta(1000, 2000) // first call — seeds prev

	rx, tx := c.applyNetDelta(1500, 2300)
	assert.Equal(t, int64(500), rx, "rx delta should be 500")
	assert.Equal(t, int64(300), tx, "tx delta should be 300")
}

// TestApplyNetDelta_CounterReset checks that negative deltas are clamped to 0.
func TestApplyNetDelta_CounterReset(t *testing.T) {
	c := New()
	c.applyNetDelta(5000, 8000) // seed prev

	// Simulate counter reset: new values smaller than prev
	rx, tx := c.applyNetDelta(100, 50)
	assert.Equal(t, int64(0), rx, "counter reset: rx delta must be clamped to 0")
	assert.Equal(t, int64(0), tx, "counter reset: tx delta must be clamped to 0")
}

// TestApplyNetDelta_StateAdvances verifies prev* is updated after each call
// so the next delta is computed relative to the latest sample.
func TestApplyNetDelta_StateAdvances(t *testing.T) {
	c := New()
	c.applyNetDelta(1000, 2000) // seed
	c.applyNetDelta(1500, 2300) // second sample

	// Third call: delta should be relative to 1500/2300, not 1000/2000
	rx, tx := c.applyNetDelta(1600, 2400)
	assert.Equal(t, int64(100), rx, "rx delta should be 100 (relative to 1500)")
	assert.Equal(t, int64(100), tx, "tx delta should be 100 (relative to 2300)")
}
