package incident

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/store"
)

// Detector tracks consecutive down counts per monitor and auto-creates
// a "detected" incident after 2 consecutive down results.
type Detector struct {
	q    *store.Queries
	mu   sync.Mutex
	cons map[int64]int // monitorID → consecutive down count
}

func NewDetector(q *store.Queries) *Detector {
	return &Detector{q: q, cons: make(map[int64]int)}
}

// MaybeCreateIncident should be called after every check result is written.
// Returns true if a new incident was created.
func (d *Detector) MaybeCreateIncident(ctx context.Context, monitorID int64, status string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if status != "down" {
		d.cons[monitorID] = 0
		return false, nil
	}

	d.cons[monitorID]++
	if d.cons[monitorID] < 2 {
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
