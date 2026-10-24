package pruner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/memetics19/pulse/api/store"
)

// BatchSize caps how many rows one DELETE statement removes. Each statement is
// its own short write transaction, so check-result inserts and agent ingest can
// take the SQLite writer between batches instead of waiting behind one large
// delete (for example the first pass after upgrading, or after lowering
// retention_days).
const BatchSize = 5000

// DeleteInBatches calls del with BatchSize until a call removes fewer rows than
// that, and returns the total removed.
func DeleteInBatches(ctx context.Context, del func(ctx context.Context, limit int64) (int64, error)) (int64, error) {
	var total int64
	for {
		n, err := del(ctx, BatchSize)
		total += n
		if err != nil || n < BatchSize {
			return total, err
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

type Pruner struct {
	q *store.Queries
}

func New(q *store.Queries) *Pruner { return &Pruner{q: q} }

// Run deletes check results, diagnostic bundles and 1-minute infra metrics
// older than the configured retention period (default 90 days), plus sessions
// that have already expired. A failure on one table does not stop the others.
func (p *Pruner) Run(ctx context.Context) error {
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -p.retentionDays(ctx))

	steps := []struct {
		table string
		del   func(ctx context.Context, limit int64) (int64, error)
	}{
		{"check_results", func(ctx context.Context, limit int64) (int64, error) {
			return p.q.PruneCheckResults(ctx, store.PruneCheckResultsParams{Cutoff: cutoff, BatchSize: limit})
		}},
		{"agent_diagnostics", func(ctx context.Context, limit int64) (int64, error) {
			return p.q.PruneAgentDiagnostics(ctx, store.PruneAgentDiagnosticsParams{Cutoff: cutoff, BatchSize: limit})
		}},
		{"infra_metrics_1m", func(ctx context.Context, limit int64) (int64, error) {
			return p.q.Prune1mMetrics(ctx, store.Prune1mMetricsParams{Cutoff: cutoff, BatchSize: limit})
		}},
		// Every session check rejects a session once expires_at is not after
		// now, and nothing else reads the table, so an expired row is dead.
		{"sessions", func(ctx context.Context, limit int64) (int64, error) {
			return p.q.DeleteExpiredSessions(ctx, store.DeleteExpiredSessionsParams{Cutoff: now, BatchSize: limit})
		}},
	}

	var errs []error
	for _, s := range steps {
		if _, err := DeleteInBatches(ctx, s.del); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.table, err))
		}
	}
	return errors.Join(errs...)
}

// retentionDays reads the retention_days setting, falling back to 90 when it
// is missing or not a positive integer.
func (p *Pruner) retentionDays(ctx context.Context) int {
	if v, err := p.q.GetSetting(ctx, "retention_days"); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			return n
		}
	}
	return 90
}
