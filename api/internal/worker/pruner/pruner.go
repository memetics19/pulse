package pruner

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/memetics19/pulse/api/store"
)

type Pruner struct {
	q *store.Queries
}

func New(q *store.Queries) *Pruner { return &Pruner{q: q} }

// Run deletes check_results and diagnostic bundles older than the configured
// retention period (default 90 days).
func (p *Pruner) Run(ctx context.Context) error {
	days := 90
	if v, err := p.q.GetSetting(ctx, "retention_days"); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			days = n
		}
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	if err := p.q.PruneCheckResults(ctx, cutoff); err != nil {
		log.Printf("pruner: check_results: %v", err)
		return err
	}
	if err := p.q.PruneIncidentDiagnostics(ctx, cutoff); err != nil {
		log.Printf("pruner: incident_diagnostics: %v", err)
		return err
	}
	return nil
}
