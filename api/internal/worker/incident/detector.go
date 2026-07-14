package incident

import (
	"context"
	"fmt"
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
	results, err := d.q.LatestTwoCheckResults(ctx, monitorID)
	if err != nil {
		return false, fmt.Errorf("list latest check results: %w", err)
	}
	if len(results) < 2 || results[0].Status != "down" || results[1].Status != "down" {
		return false, nil
	}

	// Check if there's already an active incident for this monitor
	incidents, err := d.q.ListActiveIncidents(ctx)
	if err != nil {
		return false, err
	}
	monIDStr := fmt.Sprintf("%d", monitorID)
	for _, inc := range incidents {
		if containsMonitorID(inc.AffectedMonitorIds, monIDStr) {
			return false, nil // already active
		}
	}

	// Suppress auto-incidents for monitors under an active maintenance window.
	if mws, err := d.q.ListActiveMaintenance(ctx); err == nil {
		for _, mw := range mws {
			if containsMonitorID(mw.AffectedMonitorIds, monIDStr) {
				return false, nil
			}
		}
	}

	name := fmt.Sprintf("Monitor %d", monitorID)
	if mon, err := d.q.GetMonitor(ctx, monitorID); err == nil && mon.Name != "" {
		name = mon.Name
	}

	_, err = d.q.CreateIncident(ctx, store.CreateIncidentParams{
		Title:              fmt.Sprintf("%s is down", name),
		Severity:           "major",
		AffectedMonitorIds: fmt.Sprintf("[%d]", monitorID),
		StartedAt:          time.Now(),
		Source:             "internal",
		ExternalID:         "",
	})
	if err != nil {
		return false, err
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
