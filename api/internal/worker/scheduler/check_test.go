package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

type stubChecker struct{ res checker.Result }

func (s *stubChecker) Check(context.Context, string, int64) checker.Result { return s.res }

// retryStub is a Retrier that records the policy the scheduler gives it.
type retryStub struct {
	stubChecker
	got *checker.Retry
}

func (r *retryStub) WithRetry(p checker.Retry) checker.Checker {
	*r.got = p
	return &r.stubChecker
}

func TestCheckAppliesMonitorRetryPolicy(t *testing.T) {
	tests := []struct {
		name             string
		retries, retryMs int64
		want             checker.Retry
	}{
		{name: "defaults", retries: 1, retryMs: 500, want: checker.Retry{Count: 1, Interval: 500 * time.Millisecond}},
		{name: "custom", retries: 4, retryMs: 2000, want: checker.Retry{Count: 4, Interval: 2 * time.Second}},
		{name: "negative reads as zero", retries: -1, retryMs: -5, want: checker.Retry{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got checker.Retry
			s := New(nil, nil, true)
			s.SetChecker("tcp", &retryStub{stubChecker: stubChecker{res: checker.Result{Status: "up"}}, got: &got})
			s.check(context.Background(), store.Monitor{Type: "tcp", Retries: tt.retries, RetryIntervalMs: tt.retryMs})
			if got != tt.want {
				t.Fatalf("retry = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCheckUsesMonitorFailuresAndRecoveries(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "m", Url: "x:80", Type: "tcp", IntervalSeconds: 60, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
		FailuresBeforeIncident: 3, RecoveriesBeforeResolve: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, checkresult.New(db, incident.NewDetector(q), nil), true)
	ctx := context.Background()

	s.SetChecker("tcp", &stubChecker{res: checker.Result{Status: "down", CheckedAt: time.Now()}})
	for i := range 3 {
		if incs, _ := q.ListActiveIncidents(ctx); len(incs) != 0 {
			t.Fatalf("incident opened after %d of 3 failures", i)
		}
		s.check(ctx, mon)
	}
	if incs, _ := q.ListActiveIncidents(ctx); len(incs) != 1 {
		t.Fatalf("3 failures opened %d incidents, want 1", len(incs))
	}

	s.SetChecker("tcp", &stubChecker{res: checker.Result{Status: "up", CheckedAt: time.Now()}})
	s.check(ctx, mon)
	if incs, _ := q.ListActiveIncidents(ctx); len(incs) != 0 {
		t.Fatal("one recovery should resolve when recoveries_before_resolve is 1")
	}
}

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
	disp := alerter.NewDispatcher(q, config.Config{})
	s := New(db, checkresult.New(db, det, disp), true)

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
	if cr, err := q.LatestCheckResult(ctx, mon.ID); err != nil || !strings.Contains(cr.ErrorMessage, "exceeded degraded threshold") {
		t.Fatalf("degraded override should record a reason, got %q (err %v)", cr.ErrorMessage, err)
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
	s := New(db, checkresult.New(db, nil, nil), true)
	// "http" must be registered for the dispatch to proceed; check() then builds
	// a per-monitor HTTP checker itself.
	s.SetChecker("http", &stubChecker{res: checker.Result{Status: "down"}})
	s.check(context.Background(), m)
	if got := latestStatus(t, q, m.ID); got != "up" {
		t.Fatalf("http check status = %q, want up", got)
	}
}

func TestCheckStoresCertDaysLeftAndStatusCode(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := makeMonitor(t, q)
	s := New(db, checkresult.New(db, nil, nil), true)
	days := 12
	s.SetChecker("tcp", &stubChecker{res: checker.Result{
		Status: "degraded", ResponseTimeMs: 10, StatusCode: days, CertDaysLeft: &days, CheckedAt: time.Now(),
	}})

	s.check(context.Background(), mon)

	cr, err := q.LatestCheckResult(context.Background(), mon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cr.CertDaysLeft == nil || *cr.CertDaysLeft != 12 {
		t.Fatalf("cert_days_left = %v, want 12", cr.CertDaysLeft)
	}
	if cr.StatusCode == nil || *cr.StatusCode != 12 {
		t.Fatalf("status_code = %v, want 12 (kept for one release)", cr.StatusCode)
	}
}

func TestCheckSSLTypeBuildsChecker(t *testing.T) {
	// httptest's certificate is not in the system roots, so the per-monitor
	// SSL checker that check() builds fails verification and records down.
	// The registered stub would have recorded up.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	db := testutil.NewTestDB(t)
	q := store.New(db)
	m, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "cert", Url: srv.Listener.Addr().String(), Type: "ssl", IntervalSeconds: 3600, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
		TlsWarnDays: 30, TlsCriticalDays: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, checkresult.New(db, nil, nil), true)
	s.SetChecker("ssl", &stubChecker{res: checker.Result{Status: "up", CheckedAt: time.Now()}})
	s.check(context.Background(), m)

	cr, err := q.LatestCheckResult(context.Background(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cr.Status != "down" || cr.CertDaysLeft != nil {
		t.Fatalf("ssl check = %q with cert_days_left %v, want down with none", cr.Status, cr.CertDaysLeft)
	}
}

func TestStartLaunchesAndReconciles(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	makeMonitor(t, q)

	s := New(db, checkresult.New(db, nil, nil), true)
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
	if err := New(nil, nil, true).Start(context.Background()); err == nil {
		t.Fatal("Start with no DB should error")
	}
	// Start without a recorder -> error path.
	if err := New(db, nil, true).Start(context.Background()); err == nil {
		t.Fatal("Start with no recorder should error")
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
