package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

type stubChecker struct{ res checker.Result }

func (s *stubChecker) Check(context.Context, string, int64) checker.Result { return s.res }

func makeMonitor(t *testing.T, q *store.Queries) store.Monitor {
	t.Helper()
	m, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "m", Url: "x:80", Type: "tcp", IntervalSeconds: 60, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCheckPersistsAndAppliesThresholds(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := makeMonitor(t, q)

	det := incident.NewDetector(q)
	disp := alerter.NewDispatcher(q, "", "")
	s := New(db, disp, det, true)

	var beats int
	s.SetHeartbeat(func() { beats++ })

	ctx := context.Background()

	// up within thresholds -> stored "up"
	s.SetChecker("tcp", &stubChecker{res: checker.Result{Status: "up", ResponseTimeMs: 50, CheckedAt: time.Now()}})
	s.check(ctx, mon)
	if got := latestStatus(t, q, mon.ID); got != "up" {
		t.Fatalf("status = %q, want up", got)
	}

	// up but slow -> degraded via threshold override
	s.SetChecker("tcp", &stubChecker{res: checker.Result{Status: "up", ResponseTimeMs: 800, CheckedAt: time.Now()}})
	s.check(ctx, mon)
	if got := latestStatus(t, q, mon.ID); got != "degraded" {
		t.Fatalf("status = %q, want degraded", got)
	}

	// up but very slow -> down, and two downs open an incident (exercises the
	// detector + alerter branch).
	s.SetChecker("tcp", &stubChecker{res: checker.Result{Status: "up", ResponseTimeMs: 3000, CheckedAt: time.Now()}})
	s.check(ctx, mon)
	s.check(ctx, mon)
	if got := latestStatus(t, q, mon.ID); got != "down" {
		t.Fatalf("status = %q, want down", got)
	}
	incs, _ := q.ListActiveIncidents(ctx)
	if len(incs) == 0 {
		t.Fatal("two down checks should have opened an incident")
	}

	// unknown type -> no checker, returns without persisting/panicking
	badMon := mon
	badMon.Type = "does-not-exist"
	s.check(ctx, badMon)
}

func TestCheckHTTPTypeBuildsChecker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	db := testutil.NewTestDB(t)
	q := store.New(db)
	m, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "web", Url: srv.URL, Type: "http", IntervalSeconds: 60, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, nil, nil, true)
	// "http" must be registered for the dispatch to proceed; check() then builds
	// a per-monitor HTTP checker itself.
	s.SetChecker("http", &stubChecker{res: checker.Result{Status: "down"}})
	s.check(context.Background(), m)
	if got := latestStatus(t, q, m.ID); got != "up" {
		t.Fatalf("http check status = %q, want up", got)
	}
}

func TestStartLaunchesAndReconciles(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	makeMonitor(t, q)

	s := New(db, nil, nil, true)
	s.SetChecker("tcp", &stubChecker{res: checker.Result{Status: "up", ResponseTimeMs: 10, CheckedAt: time.Now()}})

	var beats int
	s.SetHeartbeat(func() { beats++ })

	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if beats == 0 {
		t.Fatal("Start should have beat at least once via reconcile")
	}
	cancel()
	time.Sleep(20 * time.Millisecond)

	// Start with a nil DB scheduler -> error path.
	if err := New(nil, nil, nil, true).Start(context.Background()); err == nil {
		t.Fatal("Start with no DB should error")
	}
}

func latestStatus(t *testing.T, q *store.Queries, monitorID int64) string {
	t.Helper()
	cr, err := q.LatestCheckResult(context.Background(), monitorID)
	if err != nil {
		t.Fatalf("latest check result: %v", err)
	}
	return cr.Status
}
