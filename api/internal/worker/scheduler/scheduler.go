package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/store"
)

type clock interface {
	Now() time.Time
	NewTimer(time.Duration) timer
}

type timer interface {
	C() <-chan time.Time
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) timer {
	return realTimer{Timer: time.NewTimer(d)}
}

type realTimer struct {
	*time.Timer
}

func (t realTimer) C() <-chan time.Time { return t.Timer.C }

const (
	pushDeadlineGrace = 5 * time.Second
	pushRetryDelay    = time.Second

	// maxStartupJitter caps how long the startup pass delays a first check.
	maxStartupJitter = 30 * time.Second
)

// Scheduler manages one goroutine per active monitor.
type Scheduler struct {
	db           *sql.DB
	q            *store.Queries
	recorder     *checkresult.Recorder
	clock        clock
	checkers     map[string]checker.Checker
	allowPrivate bool
	beat         func() // called after each successful reconcile (worker liveness)
	mu           sync.RWMutex
	running      map[int64]runningMonitor
	started      bool // set after the first successful reconcile
}

// SetHeartbeat registers a callback invoked after every successful reconcile,
// letting /healthz observe that the worker loop is alive.
func (s *Scheduler) SetHeartbeat(beat func()) { s.beat = beat }

// runningMonitor tracks a live per-monitor goroutine. fingerprint captures
// the check-relevant fields so reconcile can restart the goroutine when the
// monitor's configuration changes.
type runningMonitor struct {
	cancel      context.CancelFunc
	fingerprint string
}

// fingerprint serializes the monitor fields that affect how checks run. The
// failure and recovery counts are in it because the running goroutine hands
// its copy of the monitor to the recorder, which passes them to the detector.
func fingerprint(m store.Monitor) string {
	expected := int64(0)
	if m.ExpectedStatus != nil {
		expected = *m.ExpectedStatus
	}
	return fmt.Sprintf("%s|%s|%d|%d|%d|%s|%d|%d|%d|%d|%d|%d|%d|%d",
		m.Url, m.Type, m.IntervalSeconds, m.TimeoutSeconds,
		expected, m.KeywordCheck, m.DegradedThresholdMs, m.DownThresholdMs,
		m.FailuresBeforeIncident, m.RecoveriesBeforeResolve, m.Retries, m.RetryIntervalMs,
		m.TlsWarnDays, m.TlsCriticalDays)
}

func New(db *sql.DB, recorder *checkresult.Recorder, allowPrivate bool) *Scheduler {
	s := &Scheduler{
		db:           db,
		recorder:     recorder,
		clock:        realClock{},
		checkers:     map[string]checker.Checker{},
		allowPrivate: allowPrivate,
		running:      map[int64]runningMonitor{},
	}
	if db != nil {
		s.q = store.New(db)
	}
	return s
}

// SetChecker registers a checker for a monitor type (e.g. "http", "tcp").
func (s *Scheduler) SetChecker(monType string, c checker.Checker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers[monType] = c
}

// RunMonitor runs a single monitor until ctx is cancelled. Polling monitors
// check immediately and then at each interval; push monitors wait for heartbeat
// deadlines instead.
func (s *Scheduler) RunMonitor(ctx context.Context, mon store.Monitor) {
	s.runMonitor(ctx, mon, false)
}

// runMonitor is RunMonitor with an optional startup jitter. With jitter set, a
// polling monitor waits firstCheckDelay before its first check, so a restart
// does not fire every monitor in the same instant.
func (s *Scheduler) runMonitor(ctx context.Context, mon store.Monitor, jitter bool) {
	if mon.Type == "push" {
		s.runPushMonitor(ctx, mon)
		return
	}

	interval := pollInterval(mon)
	if jitter {
		if d := firstCheckDelay(mon.ID, interval); d > 0 && !s.wait(ctx, d) {
			return
		}
	}
	s.check(ctx, mon)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.check(ctx, mon)
		}
	}
}

// pollInterval is the monitor's check interval, defaulting to one minute.
func pollInterval(mon store.Monitor) time.Duration {
	interval := time.Duration(mon.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return interval
}

// firstCheckDelay is a stable offset in [0, min(interval, maxStartupJitter))
// derived from the monitor ID. The same monitor keeps the same phase across
// restarts, and monitors with the same interval spread across the window
// instead of staying phase-locked.
func firstCheckDelay(id int64, interval time.Duration) time.Duration {
	window := min(interval, maxStartupJitter)
	if window <= 0 {
		return 0
	}
	// Fibonacci hashing scatters consecutive IDs across the window.
	h := uint64(id) * 0x9E3779B97F4A7C15
	return time.Duration(h % uint64(window))
}

func (s *Scheduler) runPushMonitor(ctx context.Context, mon store.Monitor) {
	if s.q == nil {
		log.Printf("scheduler: push watchdog has no database connection (monitor %d)", mon.ID)
		return
	}
	if s.recorder == nil {
		log.Printf("scheduler: push watchdog has no check result recorder (monitor %d)", mon.ID)
		return
	}

	interval := pollInterval(mon)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		base := mon.CreatedAt
		latest, err := s.q.LatestCheckResult(ctx, mon.ID)
		switch {
		case err == nil:
			base = latest.CheckedAt
		case errors.Is(err, sql.ErrNoRows):
		case ctx.Err() != nil:
			return
		default:
			log.Printf("scheduler: load latest push result (monitor %d): %v", mon.ID, err)
			if !s.wait(ctx, pushRetryDelay) {
				return
			}
			continue
		}

		deadline := base.Add(interval + pushDeadlineGrace)
		wait := deadline.Sub(s.clock.Now())
		if wait < 0 {
			wait = 0
		}
		if !s.wait(ctx, wait) {
			return
		}

		latest, err = s.q.LatestCheckResult(ctx, mon.ID)
		switch {
		case err == nil && latest.CheckedAt.After(base):
			continue
		case err == nil:
		case errors.Is(err, sql.ErrNoRows):
		case ctx.Err() != nil:
			return
		default:
			log.Printf("scheduler: reload latest push result (monitor %d): %v", mon.ID, err)
			if !s.wait(ctx, pushRetryDelay) {
				return
			}
			continue
		}

		now := s.clock.Now()
		if now.Before(deadline) {
			continue
		}
		if err := s.recorder.Record(ctx, mon, checkresult.Input{
			Status:       "down",
			ErrorMessage: "no heartbeat received before deadline",
			CheckedAt:    now,
		}); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("scheduler: record missed push heartbeat (monitor %d): %v", mon.ID, err)
			if !s.wait(ctx, pushRetryDelay) {
				return
			}
		}
	}
}

func (s *Scheduler) wait(ctx context.Context, d time.Duration) bool {
	t := s.clock.NewTimer(d)
	select {
	case <-ctx.Done():
		if !t.Stop() {
			select {
			case <-t.C():
			default:
			}
		}
		return false
	case <-t.C():
		return true
	}
}

func (s *Scheduler) check(ctx context.Context, mon store.Monitor) {
	s.mu.RLock()
	c, ok := s.checkers[mon.Type]
	s.mu.RUnlock()
	if !ok {
		log.Printf("scheduler: no checker for type %q (monitor %d)", mon.Type, mon.ID)
		return
	}

	// HTTP(S) checks honour the monitor's expected status and keyword, so build
	// a per-monitor checker instead of using the shared default one.
	if mon.Type == "http" || mon.Type == "https" {
		expected := 0
		if mon.ExpectedStatus != nil {
			expected = int(*mon.ExpectedStatus)
		}
		c = checker.NewHTTP(expected, mon.KeywordCheck, s.allowPrivate)
	}
	// SSL checks use the monitor's own certificate expiry thresholds.
	if mon.Type == "ssl" {
		c = checker.NewSSLThresholds(int(mon.TlsWarnDays), int(mon.TlsCriticalDays), s.allowPrivate)
	}
	// HTTP, TCP and TLS checks retry a transient failure as the monitor says.
	if r, ok := c.(checker.Retrier); ok {
		c = r.WithRetry(retryPolicy(mon))
	}

	result := c.Check(ctx, mon.Url, mon.TimeoutSeconds)

	if s.recorder == nil {
		return
	}

	respMs := result.ResponseTimeMs
	input := checkresult.Input{
		Status:         result.Status,
		ResponseTimeMs: &respMs,
		ErrorMessage:   result.ErrorMessage,
		CheckedAt:      result.CheckedAt,
	}
	if result.StatusCode > 0 {
		code := int64(result.StatusCode)
		input.StatusCode = &code
	}
	if result.CertDaysLeft != nil {
		days := int64(*result.CertDaysLeft)
		input.CertDaysLeft = &days
	}
	if err := s.recorder.Record(ctx, mon, input); err != nil {
		log.Printf("scheduler: record check result (monitor %d): %v", mon.ID, err)
	}
}

// retryPolicy is the monitor's retries and retry interval, with negative
// values read as zero.
func retryPolicy(mon store.Monitor) checker.Retry {
	return checker.Retry{
		Count:    int(max(mon.Retries, 0)),
		Interval: time.Duration(max(mon.RetryIntervalMs, 0)) * time.Millisecond,
	}
}

// reconcileInterval is how often the scheduler re-reads active monitors to
// pick up creates, updates, and deletes made through the API.
const reconcileInterval = 30 * time.Second

// reconcile diffs the active monitors in the DB against the running
// goroutines: it starts missing monitors, restarts changed ones, and cancels
// removed or deactivated ones. The first pass, at startup, spreads first checks
// with firstCheckDelay. Later passes start a created or edited monitor at once,
// so admin changes show a result right away.
func (s *Scheduler) reconcile(ctx context.Context) error {
	monitors, err := s.q.ListActiveMonitors(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	jitter := !s.started
	seen := make(map[int64]bool, len(monitors))
	for _, mon := range monitors {
		seen[mon.ID] = true
		fp := fingerprint(mon)
		if cur, ok := s.running[mon.ID]; ok {
			if cur.fingerprint == fp {
				continue
			}
			cur.cancel() // config changed: restart with fresh settings
			log.Printf("scheduler: restarting monitor %d (config changed)", mon.ID)
		}
		mCtx, cancel := context.WithCancel(ctx)
		s.running[mon.ID] = runningMonitor{cancel: cancel, fingerprint: fp}
		go s.runMonitor(mCtx, mon, jitter)
	}

	for id, cur := range s.running {
		if !seen[id] {
			cur.cancel()
			delete(s.running, id)
			log.Printf("scheduler: stopped monitor %d (deleted or deactivated)", id)
		}
	}
	s.started = true
	if s.beat != nil {
		s.beat()
	}
	return nil
}

// Start launches goroutines for all active monitors, then keeps them in sync
// with the DB by reconciling every reconcileInterval until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	if s.q == nil {
		return fmt.Errorf("scheduler: no database connection")
	}
	if s.recorder == nil {
		return fmt.Errorf("scheduler: no check result recorder")
	}
	if err := s.reconcile(ctx); err != nil {
		return err
	}
	s.mu.RLock()
	log.Printf("scheduler: started %d monitors", len(s.running))
	s.mu.RUnlock()

	go func() {
		tick := time.NewTicker(reconcileInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := s.reconcile(ctx); err != nil {
					log.Printf("scheduler: reconcile: %v", err)
				}
			}
		}
	}()
	return nil
}
