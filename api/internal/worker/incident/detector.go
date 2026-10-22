package incident

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/store"
)

// Detector auto-creates an incident after 2 consecutive persisted down results.
type Detector struct {
	q *store.Queries
}

func NewDetector(q *store.Queries) *Detector {
	return &Detector{q: q}
}

// MaybeCreateIncident must be called after the current check result is
// persisted. It reads the two latest rows, including that current result, so
// detection survives worker restarts and is shared across processes. It
// returns true only when it creates a new incident.
func (d *Detector) MaybeCreateIncident(ctx context.Context, monitorID int64, _ string) (bool, error) {
	return d.MaybeCreateIncidentWithQueries(ctx, d.q, monitorID)
}

// MaybeCreateIncidentWithQueries evaluates and creates an incident using the
// supplied query scope. Recorder passes transaction-scoped queries so result
// persistence and the database incident decision commit or roll back together.
func (d *Detector) MaybeCreateIncidentWithQueries(ctx context.Context, q *store.Queries, monitorID int64) (bool, error) {
	results, err := q.LatestTwoCheckResults(ctx, monitorID)
	if err != nil {
		return false, fmt.Errorf("list latest check results: %w", err)
	}
	if len(results) < 2 || results[0].Status != "down" || results[1].Status != "down" {
		return false, nil
	}

	// Check if there's already an active incident for this monitor
	incidents, err := q.ListActiveIncidents(ctx)
	if err != nil {
		return false, fmt.Errorf("list active incidents: %w", err)
	}
	monIDStr := fmt.Sprintf("%d", monitorID)
	for _, inc := range incidents {
		if containsMonitorID(inc.AffectedMonitorIds, monIDStr) {
			return false, nil // already active
		}
	}

	// Suppress auto-incidents for monitors under an active maintenance window.
	mws, err := q.ListActiveMaintenance(ctx)
	if err != nil {
		return false, fmt.Errorf("list active maintenance: %w", err)
	}
	for _, mw := range mws {
		if containsMonitorID(mw.AffectedMonitorIds, monIDStr) {
			return false, nil
		}
	}

	name := fmt.Sprintf("Monitor %d", monitorID)
	if mon, err := q.GetMonitor(ctx, monitorID); err == nil && mon.Name != "" {
		name = mon.Name
	}

	_, err = q.CreateAutoIncident(ctx, store.CreateAutoIncidentParams{
		Title:              fmt.Sprintf("%s is down", name),
		AffectedMonitorIds: fmt.Sprintf("[%d]", monitorID),
		StartedAt:          time.Now(),
		ExternalID:         strconv.FormatInt(monitorID, 10),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create auto incident: %w", err)
	}
	return true, nil
}

// containsMonitorID checks if monitorID string appears in the JSON array "[1,2,3]".
func containsMonitorID(affectedJSON, monIDStr string) bool {
	if len(affectedJSON) < 3 {
		return false
	}
	// Strip brackets and pad with commas to simplify boundary matching
	inner := "," + affectedJSON[1:len(affectedJSON)-1] + ","
	return strings.Contains(inner, ","+monIDStr+",")
}
