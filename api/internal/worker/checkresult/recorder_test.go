package checkresult_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createMonitor(t *testing.T, q *store.Queries, name string) store.Monitor {
	t.Helper()
	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: name, Url: "http://example.com", Type: "http",
		IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal",
	})
	require.NoError(t, err)
	return mon
}

func int64Ptr(v int64) *int64 { return &v }

func TestRecorder_PersistsSuppliedResultFields(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q, "Fields API")
	checkedAt := time.Date(2026, time.July, 14, 10, 30, 0, 0, time.UTC)
	r := checkresult.New(db, incident.NewDetector(q), nil)

	err := r.Record(context.Background(), mon, checkresult.Input{
		Status: "up", ResponseTimeMs: int64Ptr(125), StatusCode: int64Ptr(204),
		ErrorMessage: "diagnostic", CheckedAt: checkedAt,
	})

	require.NoError(t, err)
	results, err := q.LatestTwoCheckResults(context.Background(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "up", results[0].Status)
	assert.Equal(t, int64(125), *results[0].ResponseTimeMs)
	assert.Equal(t, int64(204), *results[0].StatusCode)
	assert.Equal(t, "diagnostic", results[0].ErrorMessage)
	assert.True(t, results[0].CheckedAt.Equal(checkedAt), "got %v want %v", results[0].CheckedAt, checkedAt)
}

func TestRecorder_AppliesLatencyThresholdsOnlyToUpResults(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		responseMs int64
		want       string
	}{
		{name: "up below threshold stays up", status: "up", responseMs: 500, want: "up"},
		{name: "up above degraded threshold degrades", status: "up", responseMs: 501, want: "degraded"},
		{name: "up above down threshold goes down", status: "up", responseMs: 2001, want: "down"},
		{name: "explicit down is never promoted", status: "down", responseMs: 1, want: "down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q, tt.name)
			r := checkresult.New(db, incident.NewDetector(q), nil)

			err := r.Record(context.Background(), mon, checkresult.Input{
				Status: tt.status, ResponseTimeMs: int64Ptr(tt.responseMs), CheckedAt: time.Now(),
			})

			require.NoError(t, err)
			results, err := q.LatestTwoCheckResults(context.Background(), mon.ID)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, tt.want, results[0].Status)
		})
	}
}

func TestRecorder_TwoDownResultsCreateOneIncident(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q, "Incident API")
	r := checkresult.New(db, incident.NewDetector(q), nil)
	now := time.Now()

	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{Status: "down", CheckedAt: now}))
	incidents, err := q.ListActiveIncidents(context.Background())
	require.NoError(t, err)
	assert.Empty(t, incidents)

	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{Status: "down", CheckedAt: now.Add(time.Second)}))
	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{Status: "down", CheckedAt: now.Add(2 * time.Second)}))
	incidents, err = q.ListActiveIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	assert.Equal(t, "Incident API is down", incidents[0].Title)
}

func TestRecorder_DispatchesAlertOnlyForNewIncident(t *testing.T) {
	var hits atomic.Int64
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits.Add(1)
		var payload struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
		texts = append(texts, payload.Text)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q, "Recorder API")
	_, err := q.CreateNotification(context.Background(), store.CreateNotificationParams{
		Channel: "slack", ConfigJson: `{"webhook_url":"` + srv.URL + `"}`, MonitorID: &mon.ID,
	})
	require.NoError(t, err)
	r := checkresult.New(db, incident.NewDetector(q), alerter.NewDispatcher(q, "", ""))
	now := time.Now()

	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", ErrorMessage: "first failure", CheckedAt: now,
	}))
	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", ErrorMessage: "second failure", CheckedAt: now.Add(time.Second),
	}))
	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", ErrorMessage: "third failure", CheckedAt: now.Add(2 * time.Second),
	}))

	assert.Equal(t, int64(1), hits.Load())
	require.Len(t, texts, 1)
	assert.Equal(t, ":red_circle: *[detected]* Recorder API is down\nsecond failure", texts[0])
}

func TestRecorder_RejectsInvalidInputBeforePersistence(t *testing.T) {
	tests := []struct {
		name  string
		input checkresult.Input
		want  string
	}{
		{
			name:  "invalid status",
			input: checkresult.Input{Status: "unknown", CheckedAt: time.Now()},
			want:  "status must be one of up, down, degraded",
		},
		{
			name:  "missing checked at",
			input: checkresult.Input{Status: "up"},
			want:  "checked_at is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q, tt.name)
			r := checkresult.New(db, incident.NewDetector(q), nil)

			err := r.Record(context.Background(), mon, tt.input)

			require.EqualError(t, err, tt.want)
			results, queryErr := q.LatestTwoCheckResults(context.Background(), mon.ID)
			require.NoError(t, queryErr)
			assert.Empty(t, results)
		})
	}
}

func TestRecorder_RollsBackResultWhenMaintenanceQueryFails(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q, "Maintenance failure")
	r := checkresult.New(db, incident.NewDetector(q), nil)
	now := time.Now()
	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", CheckedAt: now,
	}))
	_, err := db.ExecContext(context.Background(), `DROP TABLE maintenance_windows`)
	require.NoError(t, err)

	err = r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", CheckedAt: now.Add(time.Second),
	})

	require.ErrorContains(t, err, "list active maintenance")
	results, queryErr := q.LatestTwoCheckResults(context.Background(), mon.ID)
	require.NoError(t, queryErr)
	require.Len(t, results, 1, "failed incident evaluation must roll back the current result")
}

func TestRecorder_RollsBackResultWhenIncidentInsertFails(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q, "Incident failure")
	r := checkresult.New(db, incident.NewDetector(q), nil)
	now := time.Now()
	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", CheckedAt: now,
	}))
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER fail_incident_insert
		BEFORE INSERT ON incidents
		BEGIN
			SELECT RAISE(FAIL, 'forced incident insert failure');
		END`)
	require.NoError(t, err)

	err = r.Record(context.Background(), mon, checkresult.Input{
		Status: "down", CheckedAt: now.Add(time.Second),
	})

	require.ErrorContains(t, err, "create auto incident")
	results, queryErr := q.LatestTwoCheckResults(context.Background(), mon.ID)
	require.NoError(t, queryErr)
	require.Len(t, results, 1, "failed incident creation must roll back the current result")
}

func TestRecorder_ConcurrentInstancesCreateOneIncidentAndAlert(t *testing.T) {
	var alertHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		alertHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "concurrent.db")
	db1, err := store.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db1.Close()) })
	db2, err := store.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
	q1, q2 := store.New(db1), store.New(db2)
	mon := createMonitor(t, q1, "Concurrent API")
	_, err = q1.CreateNotification(context.Background(), store.CreateNotificationParams{
		Channel: "slack", ConfigJson: `{"webhook_url":"` + srv.URL + `"}`, MonitorID: &mon.ID,
	})
	require.NoError(t, err)
	r1 := checkresult.New(db1, incident.NewDetector(q1), alerter.NewDispatcher(q1, "", ""))
	r2 := checkresult.New(db2, incident.NewDetector(q2), alerter.NewDispatcher(q2, "", ""))
	now := time.Now()
	require.NoError(t, r1.Record(context.Background(), mon, checkresult.Input{
		Status: "down", CheckedAt: now,
	}))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, recorder := range []*checkresult.Recorder{r1, r2} {
		wg.Add(1)
		go func(offset int, r *checkresult.Recorder) {
			defer wg.Done()
			<-start
			errs <- r.Record(context.Background(), mon, checkresult.Input{
				Status: "down", CheckedAt: now.Add(time.Duration(offset+1) * time.Second),
			})
		}(i, recorder)
	}
	close(start)
	wg.Wait()
	close(errs)
	for recordErr := range errs {
		require.NoError(t, recordErr)
	}

	incidents, err := q1.ListActiveIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	assert.Equal(t, "monitor", incidents[0].Source)
	assert.Equal(t, int64(1), alertHits.Load())
}
