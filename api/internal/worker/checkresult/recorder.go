package checkresult

import (
	"context"
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
	q        *store.Queries
	detector *incident.Detector
	alerter  *alerter.Dispatcher
}

func New(q *store.Queries, d *incident.Detector, a *alerter.Dispatcher) *Recorder {
	return &Recorder{q: q, detector: d, alerter: a}
}

// Record applies monitor thresholds, persists the current result, then runs
// incident detection. Persistence must happen first because Detector reads the
// two latest rows and expects the current result to be among them.
func (r *Recorder) Record(ctx context.Context, monitor store.Monitor, in Input) error {
	status := in.Status
	if status == "up" && in.ResponseTimeMs != nil {
		if *in.ResponseTimeMs > monitor.DownThresholdMs {
			status = "down"
		} else if *in.ResponseTimeMs > monitor.DegradedThresholdMs {
			status = "degraded"
		}
	}

	_, err := r.q.InsertCheckResult(ctx, store.InsertCheckResultParams{
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
		return nil
	}
	created, err := r.detector.MaybeCreateIncident(ctx, monitor.ID, status)
	if err != nil {
		return fmt.Errorf("detect incident for monitor %d: %w", monitor.ID, err)
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
