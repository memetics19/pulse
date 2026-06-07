// Package maintenance provides a sweeper that auto-transitions maintenance
// windows based on their scheduled start and end times.
package maintenance

import (
	"context"
	"time"

	"github.com/memetics19/pulse/api/store"
)

// Sweeper transitions maintenance windows between statuses on each run:
//   - scheduled  → in_progress  when starts_at <= now
//   - in_progress → completed   when ends_at   <= now
type Sweeper struct {
	q *store.Queries
}

// New returns a Sweeper backed by the given query set.
func New(q *store.Queries) *Sweeper { return &Sweeper{q: q} }

// Run performs status transitions for all maintenance windows that are due.
// now is accepted as a parameter so callers (and tests) can control the clock.
func (s *Sweeper) Run(ctx context.Context, now time.Time) error {
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
	}

	return nil
}
