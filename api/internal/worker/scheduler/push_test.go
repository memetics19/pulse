package scheduler

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []*fakeTimer
	created chan *fakeTimer
}

type fakeTimer struct {
	clock    *fakeClock
	deadline time.Time
	c        chan time.Time
	active   bool
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now, created: make(chan *fakeTimer, 32)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) timer {
	c.mu.Lock()
	defer c.mu.Unlock()

	t := &fakeTimer{
		clock: c, deadline: c.now.Add(d), c: make(chan time.Time, 1), active: true,
	}
	c.timers = append(c.timers, t)
	if d <= 0 {
		t.active = false
		t.c <- c.now
	}
	c.created <- t
	return t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if t.active && !t.deadline.After(c.now) {
			t.active = false
			t.c <- c.now
		}
	}
}

func (t *fakeTimer) C() <-chan time.Time { return t.c }

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if !t.active {
		return false
	}
	t.active = false
	return true
}

func nextTimer(t *testing.T, c *fakeClock) *fakeTimer {
	t.Helper()
	select {
	case timer := <-c.created:
		return timer
	case <-time.After(2 * time.Second):
		t.Fatal("push watchdog did not create a timer")
		return nil
	}
}

func newPushWatchdog(t *testing.T, now time.Time, intervalSeconds int64) (*Scheduler, *fakeClock, *store.Queries, store.Monitor) {
	t.Helper()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon, err := q.CreateMonitor(t.Context(), store.CreateMonitorParams{
		Name: "push", Type: "push", IntervalSeconds: intervalSeconds,
		TimeoutSeconds: 5, DegradedThresholdMs: 500, DownThresholdMs: 2000,
		IsActive: true, Source: "internal",
	})
	require.NoError(t, err)
	mon.CreatedAt = now

	recorder := checkresult.New(db, incident.NewDetector(q), nil)
	s := New(db, recorder, true)
	clock := newFakeClock(now)
	s.clock = clock
	return s, clock, q, mon
}

type pushCountingChecker struct {
	calls int
}

func (c *pushCountingChecker) Check(context.Context, string, int64) checker.Result {
	c.calls++
	return checker.Result{Status: "up"}
}

func TestPushMonitorDoesNotInvokeChecker(t *testing.T) {
	createdAt := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, createdAt, 60)
	registered := &pushCountingChecker{}
	s.SetChecker("push", registered)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	nextTimer(t, clock)

	require.Zero(t, registered.calls)
	_, err := q.LatestCheckResult(t.Context(), mon.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestPushMonitorFirstMissUsesCreationDeadlineWithGrace(t *testing.T) {
	createdAt := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, createdAt, 60)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	deadlineTimer := nextTimer(t, clock)
	require.Equal(t, createdAt.Add(65*time.Second), deadlineTimer.deadline)
	clock.Advance(65*time.Second - time.Nanosecond)
	_, err := q.LatestCheckResult(t.Context(), mon.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	clock.Advance(time.Nanosecond)
	nextTimer(t, clock) // the next deadline is scheduled after persistence
	result, err := q.LatestCheckResult(t.Context(), mon.ID)
	require.NoError(t, err)
	require.Equal(t, "down", result.Status)
	require.Equal(t, "no heartbeat received before deadline", result.ErrorMessage)
	require.True(t, result.CheckedAt.Equal(createdAt.Add(65*time.Second)))
}

func TestPushMonitorReloadsHeartbeatBeforeDeclaringMiss(t *testing.T) {
	createdAt := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, createdAt, 60)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	firstTimer := nextTimer(t, clock)
	require.Equal(t, createdAt.Add(65*time.Second), firstTimer.deadline)

	clock.Advance(60 * time.Second)
	heartbeatAt := clock.Now()
	_, err := q.InsertCheckResult(t.Context(), store.InsertCheckResultParams{
		MonitorID: mon.ID, CheckedAt: heartbeatAt, Status: "up", ErrorMessage: "OK",
	})
	require.NoError(t, err)

	clock.Advance(5 * time.Second)
	extendedTimer := nextTimer(t, clock)
	require.Equal(t, heartbeatAt.Add(65*time.Second), extendedTimer.deadline)

	results, err := q.LatestTwoCheckResults(t.Context(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "up", results[0].Status)
}

func TestPushMonitorRepeatedMissesAreDeadlineSpacedAndCreateIncident(t *testing.T) {
	createdAt := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, createdAt, 60)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	nextTimer(t, clock)
	clock.Advance(65 * time.Second)
	secondDeadline := nextTimer(t, clock)
	require.Equal(t, createdAt.Add(130*time.Second), secondDeadline.deadline)
	require.Empty(t, clock.created, "watchdog must wait rather than hot-loop")

	results, err := q.LatestTwoCheckResults(t.Context(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	clock.Advance(65*time.Second - time.Nanosecond)
	results, err = q.LatestTwoCheckResults(t.Context(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	clock.Advance(time.Nanosecond)
	nextTimer(t, clock)
	results, err = q.LatestTwoCheckResults(t.Context(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "down", results[0].Status)
	require.Equal(t, "down", results[1].Status)

	incidents, err := q.ListIncidents(t.Context())
	require.NoError(t, err)
	require.Len(t, incidents, 1)
}

func TestPushMonitorCancellationStopsPendingDeadline(t *testing.T) {
	createdAt := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, createdAt, 60)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()

	pending := nextTimer(t, clock)
	cancel()
	<-done
	require.False(t, pending.active)

	clock.Advance(65 * time.Second)
	_, err := q.LatestCheckResult(t.Context(), mon.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestPushMonitorFutureResultDefersDeadline(t *testing.T) {
	now := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, now, 60)
	futureReceipt := now.Add(time.Hour)
	_, err := q.InsertCheckResult(t.Context(), store.InsertCheckResultParams{
		MonitorID: mon.ID, CheckedAt: futureReceipt, Status: "up", ErrorMessage: "OK",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	pending := nextTimer(t, clock)
	require.Equal(t, futureReceipt.Add(65*time.Second), pending.deadline)
	results, err := q.LatestTwoCheckResults(t.Context(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestPushMonitorNormalizesNonPositiveInterval(t *testing.T) {
	createdAt := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, _, mon := newPushWatchdog(t, createdAt, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	pending := nextTimer(t, clock)
	require.Equal(t, createdAt.Add(65*time.Second), pending.deadline)
}

func TestPushMonitorDatabaseErrorUsesBoundedRetry(t *testing.T) {
	now := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, _, mon := newPushWatchdog(t, now, 60)
	require.NoError(t, s.db.Close())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	retry := nextTimer(t, clock)
	require.Equal(t, now.Add(pushRetryDelay), retry.deadline)
	require.Empty(t, clock.created, "database errors must not hot-loop")
}

func TestPushMonitorRecorderFailureUsesBoundedRetry(t *testing.T) {
	now := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	s, clock, q, mon := newPushWatchdog(t, now, 60)
	_, err := s.db.Exec(`
		CREATE TRIGGER fail_push_watchdog
		BEFORE INSERT ON check_results
		BEGIN
			SELECT RAISE(FAIL, 'forced recorder failure');
		END;
	`)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	nextTimer(t, clock)
	clock.Advance(65 * time.Second)
	retry := nextTimer(t, clock)
	require.Equal(t, now.Add(65*time.Second+pushRetryDelay), retry.deadline)
	require.Empty(t, clock.created, "recorder errors must not hot-loop")
	_, err = q.LatestCheckResult(t.Context(), mon.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
