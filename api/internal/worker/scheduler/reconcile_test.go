package scheduler

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/require"
)

func TestPushFingerprintTracksWatchdogConfiguration(t *testing.T) {
	push := store.Monitor{ID: 1, Type: "push", IntervalSeconds: 60}
	changedInterval := push
	changedInterval.IntervalSeconds = 90
	changedType := push
	changedType.Type = "http"

	require.NotEqual(t, fingerprint(push), fingerprint(changedInterval))
	require.NotEqual(t, fingerprint(push), fingerprint(changedType))
}

func TestReconcileTracksMonitorLifecycle(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	ctx := context.Background()

	mon, err := q.CreateMonitor(ctx, store.CreateMonitorParams{
		Name: "one", Url: "https://one.example.com", Type: "http",
		IntervalSeconds: 3600, TimeoutSeconds: 5, IsActive: true,
	})
	require.NoError(t, err)

	s := New(db, nil, true)
	require.NoError(t, s.reconcile(ctx))
	s.mu.RLock()
	_, running := s.running[mon.ID]
	s.mu.RUnlock()
	require.True(t, running, "existing monitor should be scheduled")

	// A new monitor is picked up on the next reconcile.
	mon2, err := q.CreateMonitor(ctx, store.CreateMonitorParams{
		Name: "two", Url: "https://two.example.com", Type: "http",
		IntervalSeconds: 3600, TimeoutSeconds: 5, IsActive: true,
	})
	require.NoError(t, err)
	require.NoError(t, s.reconcile(ctx))
	s.mu.RLock()
	_, running = s.running[mon2.ID]
	n := len(s.running)
	s.mu.RUnlock()
	require.True(t, running, "new monitor should be scheduled")
	require.Equal(t, 2, n)

	// A changed URL restarts the goroutine with a new fingerprint.
	s.mu.RLock()
	fpBefore := s.running[mon.ID].fingerprint
	s.mu.RUnlock()
	_, err = q.UpdateMonitor(ctx, store.UpdateMonitorParams{
		ID: mon.ID, Name: mon.Name, Url: "https://changed.example.com", Type: mon.Type,
		IntervalSeconds: mon.IntervalSeconds, TimeoutSeconds: mon.TimeoutSeconds, IsActive: true,
	})
	require.NoError(t, err)
	require.NoError(t, s.reconcile(ctx))
	s.mu.RLock()
	fpAfter := s.running[mon.ID].fingerprint
	s.mu.RUnlock()
	require.NotEqual(t, fpBefore, fpAfter, "fingerprint should change with the URL")

	// A deleted monitor is cancelled and removed.
	require.NoError(t, q.DeleteMonitor(ctx, mon.ID))
	require.NoError(t, s.reconcile(ctx))
	s.mu.RLock()
	_, running = s.running[mon.ID]
	n = len(s.running)
	s.mu.RUnlock()
	require.False(t, running, "deleted monitor should be stopped")
	require.Equal(t, 1, n)
}
