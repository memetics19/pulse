package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
)

// Scheduler manages one goroutine per active monitor.
type Scheduler struct {
	db       *sql.DB
	q        *store.Queries
	alerter  *alerter.Dispatcher
	detector *incident.Detector
	checkers map[string]checker.Checker
	mu       sync.RWMutex
	cancel   map[int64]context.CancelFunc
}

func New(db *sql.DB, a *alerter.Dispatcher, d *incident.Detector) *Scheduler {
	s := &Scheduler{
		db:       db,
		alerter:  a,
		detector: d,
		checkers: map[string]checker.Checker{},
		cancel:   map[int64]context.CancelFunc{},
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
		c = checker.NewHTTP(expected, mon.KeywordCheck)
	}

	result := c.Check(ctx, mon.Url, mon.TimeoutSeconds)

	// Apply latency thresholds on top of checker's own status
	status := result.Status
	if status == "up" && result.ResponseTimeMs > mon.DownThresholdMs {
		status = "down"
	} else if status == "up" && result.ResponseTimeMs > mon.DegradedThresholdMs {
		status = "degraded"
	}

	if s.q == nil {
		return // nil DB: test mode, skip persistence
	}

	respMs := result.ResponseTimeMs
	p := store.InsertCheckResultParams{
		MonitorID:      mon.ID,
		CheckedAt:      result.CheckedAt,
		Status:         status,
		ResponseTimeMs: &respMs,
		ErrorMessage:   result.ErrorMessage,
	}
	if result.StatusCode > 0 {
		code := int64(result.StatusCode)
		p.StatusCode = &code
	}
	if _, err := s.q.InsertCheckResult(ctx, p); err != nil {
		log.Printf("scheduler: insert check result (monitor %d): %v", mon.ID, err)
	}

	if s.detector != nil {
		created, err := s.detector.MaybeCreateIncident(ctx, mon.ID, status)
		if err != nil {
			log.Printf("scheduler: incident detection (monitor %d): %v", mon.ID, err)
		}
		if created && s.alerter != nil {
			monID := mon.ID
			s.alerter.Notify(ctx, alerter.Alert{
				Title:    fmt.Sprintf("%s is down", mon.Name),
				Status:   "detected",
				Severity: "major",
				Message:  result.ErrorMessage,
			}, &monID)
		}
	}
}

// Start loads all active monitors from DB and launches a goroutine for each.
func (s *Scheduler) Start(ctx context.Context) error {
	if s.q == nil {
		return fmt.Errorf("scheduler: no database connection")
	}
	monitors, err := s.q.ListActiveMonitors(ctx)
	if err != nil {
		return err
	}
	for _, mon := range monitors {
		mCtx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.cancel[mon.ID] = cancel
		s.mu.Unlock()
		go s.RunMonitor(mCtx, mon)
	}
	log.Printf("scheduler: started %d monitors", len(monitors))
	return nil
}
