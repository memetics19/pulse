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

// Defaults for a monitor's failures_before_incident and
// recoveries_before_resolve. They also apply when a stored value is not
// positive.
const (
	DefaultFailuresBeforeIncident  = 2
	DefaultRecoveriesBeforeResolve = 2
)

// Detector auto-creates an incident after a monitor's configured number of
// consecutive persisted down results, and resolves it after the configured
// number of answering results.
type Detector struct {
	q *store.Queries
}

func NewDetector(q *store.Queries) *Detector {
	return &Detector{q: q}
}

// MaybeCreateIncident must be called after the current check result is
// persisted. It applies DefaultFailuresBeforeIncident to the latest rows,
// including that current result, so detection survives worker restarts and is
// shared across processes. It returns true only when it creates a new
// incident.
func (d *Detector) MaybeCreateIncident(ctx context.Context, monitorID int64, _ string) (bool, error) {
	return d.MaybeCreateIncidentWithQueries(ctx, d.q, monitorID, DefaultFailuresBeforeIncident)
}

// MaybeCreateIncidentWithQueries evaluates and creates an incident using the
// supplied query scope. It opens one only when the latest failures results
// are all down. Recorder passes transaction-scoped queries so result persistence and the
// database incident decision commit or roll back together.
func (d *Detector) MaybeCreateIncidentWithQueries(ctx context.Context, q *store.Queries, monitorID, failures int64) (bool, error) {
	down, err := latestAll(ctx, q, monitorID, orDefault(failures, DefaultFailuresBeforeIncident), isDown)
	if err != nil || !down {
		return false, err
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
		StartedAt:          time.Now().UTC(),
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

// MaybeResolveIncidentsWithQueries resolves the detector's own open incidents
// for a monitor once its latest recoveries persisted results are all up or
// degraded. Like MaybeCreateIncidentWithQueries it must run after the current
// result is persisted and in the same query scope, so the decision survives
// restarts and commits together with the result. It skips any incident a
// person touched: one moved past detected, one with timeline updates, or one
// that covers other monitors. It returns the incidents it resolved, as stored
// after resolving.
func (d *Detector) MaybeResolveIncidentsWithQueries(ctx context.Context, q *store.Queries, monitorID, recoveries int64) ([]store.Incident, error) {
	recovered, err := latestAll(ctx, q, monitorID, orDefault(recoveries, DefaultRecoveriesBeforeResolve), answers)
	if err != nil || !recovered {
		return nil, err
	}

	incidents, err := q.ListAutoResolvableIncidents(ctx, store.ListAutoResolvableIncidentsParams{
		ExternalID:         strconv.FormatInt(monitorID, 10),
		AffectedMonitorIds: fmt.Sprintf("[%d]", monitorID),
	})
	if err != nil {
		return nil, fmt.Errorf("list auto-resolvable incidents: %w", err)
	}

	now := time.Now().UTC()
	resolved := make([]store.Incident, 0, len(incidents))
	for _, inc := range incidents {
		rca := fmt.Sprintf("Recovered by itself at %s after %s. No root cause was recorded. Edit this if you know it.",
			now.Format(time.RFC3339), humanDuration(now.Sub(inc.StartedAt)))
		updated, err := q.UpdateIncidentStatus(ctx, store.UpdateIncidentStatusParams{
			Status: "resolved", ResolvedAt: &now, Rca: &rca, ID: inc.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("resolve incident %d: %w", inc.ID, err)
		}
		if _, err := q.CreateIncidentUpdate(ctx, store.CreateIncidentUpdateParams{
			IncidentID: inc.ID, Status: "resolved", Message: "Recovered by itself.", Author: "Pulse",
		}); err != nil {
			return nil, fmt.Errorf("add resolve update to incident %d: %w", inc.ID, err)
		}
		resolved = append(resolved, updated)
	}
	return resolved, nil
}

// latestAll reports whether the monitor has at least n persisted results and
// the n latest all match.
func latestAll(ctx context.Context, q *store.Queries, monitorID, n int64, match func(string) bool) (bool, error) {
	statuses, err := q.LatestCheckStatuses(ctx, store.LatestCheckStatusesParams{MonitorID: monitorID, N: n})
	if err != nil {
		return false, fmt.Errorf("list latest check results: %w", err)
	}
	if int64(len(statuses)) < n {
		return false, nil
	}
	for _, s := range statuses {
		if !match(s) {
			return false, nil
		}
	}
	return true, nil
}

// orDefault returns n, or def when n is not positive.
func orDefault(n, def int64) int64 {
	if n < 1 {
		return def
	}
	return n
}

// isDown reports whether a check status is a failure.
func isDown(status string) bool {
	return status == "down"
}

// answers reports whether a check status means the monitor responded.
func answers(status string) bool {
	return status == "up" || status == "degraded"
}

// humanDuration formats d for people, such as 45s, 7m or 1h 19m.
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(max(d, 0).Seconds()))
	}
	d = d.Round(time.Minute)
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
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
