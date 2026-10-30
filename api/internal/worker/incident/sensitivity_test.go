package incident_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// detect records one result and runs detection the way the recorder does,
// with a fresh detector so only persisted rows drive the decision.
func detect(t *testing.T, q *store.Queries, monitorID int64, status string, at time.Time, failures int64) bool {
	t.Helper()
	insertResult(t, q, monitorID, status, at)
	created, err := incident.NewDetector(q).MaybeCreateIncidentWithQueries(context.Background(), q, monitorID, failures)
	require.NoError(t, err)
	return created
}

func TestDetector_OpensOneIncidentAfterConfiguredFailures(t *testing.T) {
	for _, n := range []int64{1, 2, 3, 5, 10} {
		t.Run(fmt.Sprintf("%d failures", n), func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q)
			now := time.Now()

			for i := range n - 1 {
				assert.False(t, detect(t, q, mon.ID, "down", now.Add(time.Duration(i)*time.Second), n),
					"%d of %d failures must not open an incident", i+1, n)
			}
			active, err := q.ListActiveIncidents(context.Background())
			require.NoError(t, err)
			assert.Empty(t, active, "N-1 failures open nothing")

			assert.True(t, detect(t, q, mon.ID, "down", now.Add(time.Duration(n)*time.Second), n))
			// More failures keep the same incident open instead of adding one.
			assert.False(t, detect(t, q, mon.ID, "down", now.Add(time.Duration(n+1)*time.Second), n))

			active, err = q.ListActiveIncidents(context.Background())
			require.NoError(t, err)
			assert.Len(t, active, 1)
		})
	}
}

func TestDetector_AnswerResetsFailureCount(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()

	assert.False(t, detect(t, q, mon.ID, "down", now, 3))
	assert.False(t, detect(t, q, mon.ID, "down", now.Add(time.Second), 3))
	assert.False(t, detect(t, q, mon.ID, "degraded", now.Add(2*time.Second), 3))
	assert.False(t, detect(t, q, mon.ID, "down", now.Add(3*time.Second), 3))
	assert.False(t, detect(t, q, mon.ID, "down", now.Add(4*time.Second), 3))
	assert.True(t, detect(t, q, mon.ID, "down", now.Add(5*time.Second), 3))
}

func TestDetector_NonPositiveCountsUseDefaults(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()

	assert.False(t, detect(t, q, mon.ID, "down", now, 0), "0 means the default of 2")
	assert.True(t, detect(t, q, mon.ID, "down", now.Add(time.Second), 0))

	insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
	n, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, mon.ID, -1)
	require.NoError(t, err)
	assert.Empty(t, n, "-1 means the default of 2")
	insertResult(t, q, mon.ID, "up", now.Add(3*time.Second))
	n, err = incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, mon.ID, -1)
	require.NoError(t, err)
	assert.Len(t, n, 1)
}

func TestResolve_ResolvesAfterConfiguredRecoveries(t *testing.T) {
	for _, n := range []int64{1, 2, 3, 5, 10} {
		t.Run(fmt.Sprintf("%d recoveries", n), func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q)
			now := time.Now()
			inc := openAutoIncident(t, q, mon, now)

			for i := range n - 1 {
				insertResult(t, q, mon.ID, "up", now.Add(time.Duration(i+2)*time.Second))
				got, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, mon.ID, n)
				require.NoError(t, err)
				assert.Empty(t, got, "%d of %d recoveries must not resolve", i+1, n)
			}
			assert.Equal(t, "detected", getIncident(t, q, inc.ID).Status, "N-1 recoveries leave it open")

			insertResult(t, q, mon.ID, "up", now.Add(time.Duration(n+2)*time.Second))
			got, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, mon.ID, n)
			require.NoError(t, err)
			assert.Len(t, got, 1)
			assert.Equal(t, "resolved", getIncident(t, q, inc.ID).Status)
		})
	}
}

func TestResolve_FailureResetsRecoveryCount(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	inc := openAutoIncident(t, q, mon, now)

	statuses := []string{"up", "degraded", "down", "up", "up"}
	for i, s := range statuses {
		insertResult(t, q, mon.ID, s, now.Add(time.Duration(i+2)*time.Second))
		got, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, mon.ID, 3)
		require.NoError(t, err)
		assert.Empty(t, got, "after %v", statuses[:i+1])
	}
	assert.Equal(t, "detected", getIncident(t, q, inc.ID).Status)

	insertResult(t, q, mon.ID, "degraded", now.Add(10*time.Second))
	got, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, mon.ID, 3)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}
