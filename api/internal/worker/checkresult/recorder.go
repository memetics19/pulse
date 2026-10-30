package checkresult

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/memetics19/pulse/api/internal/metrics"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
)

type Input struct {
	Status         string
	ResponseTimeMs *int64
	StatusCode     *int64
	ErrorMessage   string
	CheckedAt      time.Time
}

type Recorder struct {
	db       *sql.DB
	detector *incident.Detector
	alerter  *alerter.Dispatcher
}

func New(db *sql.DB, d *incident.Detector, a *alerter.Dispatcher) *Recorder {
	return &Recorder{db: db, detector: d, alerter: a}
}

// Record applies monitor thresholds, persists the current result, then runs
// incident detection and auto-resolve. Persistence must happen first because
// Detector reads the latest rows and expects the current result to be among
// them. The monitor's failures_before_incident and recoveries_before_resolve
// say how many rows it reads.
func (r *Recorder) Record(ctx context.Context, monitor store.Monitor, in Input) error {
	if in.Status != "up" && in.Status != "down" && in.Status != "degraded" {
		return fmt.Errorf("status must be one of up, down, degraded")
	}
	if in.CheckedAt.IsZero() {
		return fmt.Errorf("checked_at is required")
	}

	// Apply latency thresholds on top of the reported status. Record why the
	// status was overridden so the UI shows a reason rather than a bare "down".
	status := in.Status
	errMsg := in.ErrorMessage
	if status == "up" && in.ResponseTimeMs != nil {
		if *in.ResponseTimeMs > monitor.DownThresholdMs {
			status = "down"
			errMsg = fmt.Sprintf("response time %dms exceeded down threshold %dms", *in.ResponseTimeMs, monitor.DownThresholdMs)
		} else if *in.ResponseTimeMs > monitor.DegradedThresholdMs {
			status = "degraded"
			errMsg = fmt.Sprintf("response time %dms exceeded degraded threshold %dms", *in.ResponseTimeMs, monitor.DegradedThresholdMs)
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin check result transaction: %w", err)
	}
	// Rollback is a no-op once Commit succeeds; on failure the original
	// error is the one returned.
	defer func() { _ = tx.Rollback() }()
	q := store.New(tx)

	_, err = q.InsertCheckResult(ctx, store.InsertCheckResultParams{
		MonitorID:      monitor.ID,
		CheckedAt:      in.CheckedAt.UTC(),
		Status:         status,
		ResponseTimeMs: in.ResponseTimeMs,
		StatusCode:     in.StatusCode,
		ErrorMessage:   errMsg,
	})
	if err != nil {
		return fmt.Errorf("insert check result for monitor %d: %w", monitor.ID, err)
	}

	if r.detector == nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit check result transaction: %w", err)
		}
		metrics.ObserveCheck(monitor.Name, status)
		return nil
	}
	created, err := r.detector.MaybeCreateIncidentWithQueries(ctx, q, monitor.ID, monitor.FailuresBeforeIncident)
	if err != nil {
		return fmt.Errorf("detect incident for monitor %d: %w", monitor.ID, err)
	}
	if _, err := r.detector.MaybeResolveIncidentsWithQueries(ctx, q, monitor.ID, monitor.RecoveriesBeforeResolve); err != nil {
		return fmt.Errorf("resolve recovered incidents for monitor %d: %w", monitor.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit check result transaction: %w", err)
	}
	metrics.ObserveCheck(monitor.Name, status)
	if created && r.alerter != nil {
		monitorID := monitor.ID
		r.alerter.Notify(ctx, alerter.Alert{
			Title:    fmt.Sprintf("%s is down", monitor.Name),
			Status:   "detected",
			Severity: "major",
			Message:  in.ErrorMessage,
		}, &monitorID)
	}
	return nil
}
