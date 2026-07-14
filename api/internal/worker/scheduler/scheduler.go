package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/store"
)

// Scheduler manages one goroutine per active monitor.
type Scheduler struct {
	db           *sql.DB
	q            *store.Queries
	recorder     *checkresult.Recorder
	checkers     map[string]checker.Checker
	allowPrivate bool
	mu           sync.RWMutex
	running      map[int64]runningMonitor
}

// runningMonitor tracks a live per-monitor goroutine. fingerprint captures
// the check-relevant fields so reconcile can restart the goroutine when the
// monitor's configuration changes.
type runningMonitor struct {
	cancel      context.CancelFunc
	fingerprint string
}

// fingerprint serializes the monitor fields that affect how checks run.
func fingerprint(m store.Monitor) string {
	expected := int64(0)
	if m.ExpectedStatus != nil {
		expected = *m.ExpectedStatus
	}
	return fmt.Sprintf("%s|%s|%d|%d|%d|%s|%d|%d",
		m.Url, m.Type, m.IntervalSeconds, m.TimeoutSeconds,
		expected, m.KeywordCheck, m.DegradedThresholdMs, m.DownThresholdMs)
}

func New(db *sql.DB, recorder *checkresult.Recorder, allowPrivate bool) *Scheduler {
	s := &Scheduler{
		db:           db,
		recorder:     recorder,
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

// RunMonitor runs a single monitor in a loop until ctx is cancelled.
// It checks immediately on start, then at each interval.
func (s *Scheduler) RunMonitor(ctx context.Context, mon store.Monitor) {
	interval := time.Duration(mon.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
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
	if err := s.recorder.Record(ctx, mon, input); err != nil {
		log.Printf("scheduler: record check result (monitor %d): %v", mon.ID, err)
	}
}

// reconcileInterval is how often the scheduler re-reads active monitors to
// pick up creates, updates, and deletes made through the API.
const reconcileInterval = 30 * time.Second

// reconcile diffs the active monitors in the DB against the running
// goroutines: it starts missing monitors, restarts changed ones, and cancels
// removed or deactivated ones.
func (s *Scheduler) reconcile(ctx context.Context) error {
	monitors, err := s.q.ListActiveMonitors(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

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
		go s.RunMonitor(mCtx, mon)
	}

	for id, cur := range s.running {
		if !seen[id] {
			cur.cancel()
			delete(s.running, id)
			log.Printf("scheduler: stopped monitor %d (deleted or deactivated)", id)
		}
	}
	return nil
}

// Start launches goroutines for all active monitors, then keeps them in sync
// with the DB by reconciling every reconcileInterval until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	if s.q == nil {
		return fmt.Errorf("scheduler: no database connection")
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
