// Package maintenance provides a sweeper that auto-transitions maintenance
// windows based on their scheduled start and end times.
package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/store"
)

// Notifier sends an alert to the notification channels. *alerter.Dispatcher
// implements it.
type Notifier interface {
	Notify(ctx context.Context, a alerter.Alert, monitorIDs ...int64)
}

// Sweeper transitions maintenance windows between statuses on each run:
//   - scheduled  → in_progress  when starts_at <= now
//   - in_progress → completed   when ends_at   <= now
type Sweeper struct {
	q      *store.Queries
	notify Notifier
}

// New returns a Sweeper backed by the given query set.
func New(q *store.Queries) *Sweeper { return &Sweeper{q: q} }

// WithNotifier makes the sweeper alert the notification channels when a window
// starts and when it ends.
func (s *Sweeper) WithNotifier(n Notifier) *Sweeper {
	s.notify = n
	return s
}

func (s *Sweeper) alert(ctx context.Context, m store.MaintenanceWindow, status, msg string) {
	if s.notify == nil {
		return
	}
	s.notify.Notify(ctx, alerter.Alert{
		Title:    "Maintenance: " + m.Title,
		Status:   status,
		Severity: "maintenance",
		Message:  msg,
	}, alerter.MonitorIDs(m.AffectedMonitorIds)...)
}

// Run performs status transitions for all maintenance windows that are due.
// now is accepted as a parameter so callers (and tests) can control the clock.
// It is converted to UTC because windows are stored in UTC and SQLite compares
// the stored text.
func (s *Sweeper) Run(ctx context.Context, now time.Time) error {
	now = now.UTC()
	starting, err := s.q.DueToStart(ctx, now)
	if err != nil {
		return err
	}
	for _, m := range starting {
		if err := s.q.UpdateMaintenanceStatus(ctx, store.UpdateMaintenanceStatusParams{
			Status: "in_progress",
			ID:     m.ID,
		}); err != nil {
			return err
		}
		s.alert(ctx, m, "in_progress", fmt.Sprintf("Scheduled maintenance started. It is planned to end at %s.", m.EndsAt.UTC().Format(time.RFC3339)))
	}

	ending, err := s.q.DueToEnd(ctx, now)
	if err != nil {
		return err
	}
	for _, m := range ending {
		if err := s.q.UpdateMaintenanceStatus(ctx, store.UpdateMaintenanceStatusParams{
			Status: "completed",
			ID:     m.ID,
		}); err != nil {
			return err
		}
		s.alert(ctx, m, "completed", "Scheduled maintenance is complete.")
	}

	return nil
}
