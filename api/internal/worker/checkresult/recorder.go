package checkresult

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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
// incident detection. Persistence must happen first because Detector reads the
// two latest rows and expects the current result to be among them.
func (r *Recorder) Record(ctx context.Context, monitor store.Monitor, in Input) error {
	if in.Status != "up" && in.Status != "down" && in.Status != "degraded" {
		return fmt.Errorf("status must be one of up, down, degraded")
	}
	if in.CheckedAt.IsZero() {
		return fmt.Errorf("checked_at is required")
	}

	status := in.Status
	if status == "up" && in.ResponseTimeMs != nil {
		if *in.ResponseTimeMs > monitor.DownThresholdMs {
			status = "down"
		} else if *in.ResponseTimeMs > monitor.DegradedThresholdMs {
			status = "degraded"
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin check result transaction: %w", err)
	}
	defer tx.Rollback()
	q := store.New(tx)

	_, err = q.InsertCheckResult(ctx, store.InsertCheckResultParams{
		MonitorID:      monitor.ID,
		CheckedAt:      in.CheckedAt,
		Status:         status,
		ResponseTimeMs: in.ResponseTimeMs,
		StatusCode:     in.StatusCode,
		ErrorMessage:   in.ErrorMessage,
	})
	if err != nil {
		return fmt.Errorf("insert check result for monitor %d: %w", monitor.ID, err)
	}

	if r.detector == nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit check result transaction: %w", err)
		}
		return nil
	}
	created, err := r.detector.MaybeCreateIncidentWithQueries(ctx, q, monitor.ID)
	if err != nil {
		return fmt.Errorf("detect incident for monitor %d: %w", monitor.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit check result transaction: %w", err)
	}
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
