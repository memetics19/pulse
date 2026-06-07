package rollup

import (
	"context"
	"log"
	"time"

	"github.com/memetics19/pulse/api/store"
)

type Rollup struct {
	q *store.Queries
}

func New(q *store.Queries) *Rollup { return &Rollup{q: q} }

// Run aggregates raw metrics into 1-minute buckets and prunes raw data older than 24h.
func (r *Rollup) Run(ctx context.Context) error {
	agents, err := r.q.ListAgents(ctx)
	if err != nil {
		return err
	}

	since := time.Now().Add(-25 * time.Hour)

	type key struct {
		agentID  int64
		bucketAt time.Time
	}
	type agg struct {
		cpuSum, memSum, diskSum float64
		rxSum, txSum            int64
		count                   int64
	}
	buckets := map[key]*agg{}

	for _, agent := range agents {
		raws, err := r.q.ListRawMetrics(ctx, store.ListRawMetricsParams{
			AgentID:     agent.ID,
			CollectedAt: since,
		})
		if err != nil {
			log.Printf("rollup: list raw metrics for agent %d: %v", agent.ID, err)
			continue
		}
		for _, raw := range raws {
			k := key{
				agentID:  raw.AgentID,
				bucketAt: raw.CollectedAt.Truncate(time.Minute),
			}
			a := buckets[k]
			if a == nil {
				a = &agg{}
				buckets[k] = a
			}
			a.cpuSum += raw.CpuPercent
			a.memSum += raw.MemPercent
			a.diskSum += raw.DiskPercent
			a.rxSum += raw.NetRxBytes
			a.txSum += raw.NetTxBytes
			a.count++
		}
	}

	for k, a := range buckets {
		if a.count == 0 {
			continue
		}
		if err := r.q.Upsert1mMetric(ctx, store.Upsert1mMetricParams{
			AgentID:     k.agentID,
			BucketAt:    k.bucketAt,
			CpuPercent:  a.cpuSum / float64(a.count),
			MemPercent:  a.memSum / float64(a.count),
			DiskPercent: a.diskSum / float64(a.count),
			NetRxBytes:  a.rxSum / a.count,
			NetTxBytes:  a.txSum / a.count,
		}); err != nil {
			log.Printf("rollup: upsert 1m bucket: %v", err)
		}
	}

	return r.q.PruneRawMetrics(ctx, time.Now().Add(-24*time.Hour))
}
