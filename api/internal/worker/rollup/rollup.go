package rollup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/pruner"
	"github.com/memetics19/pulse/api/store"
)

const (
	// window is how far back raw samples are read when building buckets.
	window = 25 * time.Hour
	// rawRetention is how long raw samples are kept once rolled up.
	rawRetention = 24 * time.Hour
)

// Rollup folds raw agent samples into 1-minute averages in infra_metrics_1m.
//
// The first Run after start rebuilds every bucket in the window. Later runs
// only rebuild the buckets that received a raw row since the previous run,
// found by the raw row id watermark. Raw ids are AUTOINCREMENT and SQLite has
// one writer, so every row inserted after a run has a larger id than the
// watermark, including late samples whose collected_at lands in an old
// bucket. A rebuilt bucket is recomputed from all of its raw rows, so the
// result is the same as a full pass over the same data.
type Rollup struct {
	q *store.Queries
	// watermark is the highest infra_metrics_raw.id already rolled up. It is
	// only meaningful once primed is true.
	watermark int64
	primed    bool
}

func New(q *store.Queries) *Rollup { return &Rollup{q: q} }

// agentPlan says which of one agent's buckets to rebuild. from is the earliest
// collected_at to read. A nil buckets set means every bucket from there on.
type agentPlan struct {
	from    time.Time
	buckets map[int64]bool // keyed by bucket start in Unix seconds
}

type agg struct {
	bucketAt                time.Time
	cpuSum, memSum, diskSum float64
	rxSum, txSum            int64
	count                   int64
}

// Run rebuilds the 1-minute buckets that can have changed since the last run
// and prunes raw data older than 24h. If any bucket fails to rebuild, the
// watermark stays put so the next run retries the same rows.
func (r *Rollup) Run(ctx context.Context) error {
	now := time.Now()
	since := now.Add(-window)

	through, err := r.q.MaxRawMetricID(ctx)
	if err != nil {
		return err
	}

	var plans map[int64]*agentPlan
	if r.primed {
		plans, err = r.changedBuckets(ctx, since, through)
	} else {
		plans, err = r.allBuckets(ctx, since)
	}
	if err != nil {
		return err
	}

	var errs []error
	for agentID, plan := range plans {
		if err := r.rebuild(ctx, agentID, plan); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		r.watermark, r.primed = through, true
	}

	if _, err := pruner.DeleteInBatches(ctx, func(ctx context.Context, limit int64) (int64, error) {
		return r.q.PruneRawMetrics(ctx, store.PruneRawMetricsParams{Cutoff: now.Add(-rawRetention), BatchSize: limit})
	}); err != nil {
		errs = append(errs, fmt.Errorf("prune raw metrics: %w", err))
	}
	return errors.Join(errs...)
}

// allBuckets plans a full rebuild of every agent's window.
func (r *Rollup) allBuckets(ctx context.Context, since time.Time) (map[int64]*agentPlan, error) {
	agents, err := r.q.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	plans := make(map[int64]*agentPlan, len(agents))
	for _, a := range agents {
		plans[a.ID] = &agentPlan{from: since}
	}
	return plans, nil
}

// changedBuckets plans a rebuild of only the buckets touched by raw rows with
// ids in (watermark, through].
func (r *Rollup) changedBuckets(ctx context.Context, since time.Time, through int64) (map[int64]*agentPlan, error) {
	plans := map[int64]*agentPlan{}
	if through <= r.watermark {
		return plans, nil
	}
	rows, err := r.q.ListRawMetricsAfterID(ctx, store.ListRawMetricsAfterIDParams{
		AfterID: r.watermark, ThroughID: through, Since: since,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		bucket := row.CollectedAt.Truncate(time.Minute)
		p := plans[row.AgentID]
		if p == nil {
			p = &agentPlan{from: bucket, buckets: map[int64]bool{}}
			plans[row.AgentID] = p
		}
		if bucket.Before(p.from) {
			p.from = bucket
		}
		p.buckets[bucket.Unix()] = true
	}
	// A full pass never reads before since, so neither does a partial one.
	for _, p := range plans {
		if p.from.Before(since) {
			p.from = since
		}
	}
	return plans, nil
}

// rebuild recomputes the planned buckets for one agent from its raw rows.
func (r *Rollup) rebuild(ctx context.Context, agentID int64, plan *agentPlan) error {
	raws, err := r.q.ListRawMetrics(ctx, store.ListRawMetricsParams{
		AgentID:     agentID,
		CollectedAt: plan.from,
	})
	if err != nil {
		return fmt.Errorf("list raw metrics for agent %d: %w", agentID, err)
	}

	buckets := map[int64]*agg{}
	var order []int64
	for _, raw := range raws {
		bucketAt := raw.CollectedAt.Truncate(time.Minute)
		k := bucketAt.Unix()
		if plan.buckets != nil && !plan.buckets[k] {
			continue
		}
		a := buckets[k]
		if a == nil {
			a = &agg{bucketAt: bucketAt}
			buckets[k] = a
			order = append(order, k)
		}
		a.cpuSum += raw.CpuPercent
		a.memSum += raw.MemPercent
		a.diskSum += raw.DiskPercent
		a.rxSum += raw.NetRxBytes
		a.txSum += raw.NetTxBytes
		a.count++
	}

	for _, k := range order {
		a := buckets[k]
		if err := r.q.Upsert1mMetric(ctx, store.Upsert1mMetricParams{
			AgentID:     agentID,
			BucketAt:    a.bucketAt,
			CpuPercent:  a.cpuSum / float64(a.count),
			MemPercent:  a.memSum / float64(a.count),
			DiskPercent: a.diskSum / float64(a.count),
			NetRxBytes:  a.rxSum / a.count,
			NetTxBytes:  a.txSum / a.count,
		}); err != nil {
			return fmt.Errorf("upsert 1m bucket for agent %d: %w", agentID, err)
		}
	}
	return nil
}
