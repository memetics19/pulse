package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
)

// workerStaleAfter is how long the worker may go without a successful reconcile
// before /healthz reports unhealthy. The reconcile loop beats every 30s, so a
// few missed beats indicate the worker has died or wedged.
const workerStaleAfter = 90 * time.Second

// Health reports process health. Once the app is configured, it also requires a
// live monitoring worker: a silently-dead worker returns 503 so orchestration
// restarts the container instead of trusting a process that monitors nothing.
type Health struct{ a *app.App }

func NewHealth(a *app.App) *Health { return &Health{a: a} }

func (h *Health) Get(w http.ResponseWriter, r *http.Request) {
	status, code := "ok", http.StatusOK
	// Before setup completes there is no worker to check; report ok so the
	// container is considered up while the operator finishes the wizard.
	if h.a.Configured() && !h.a.WorkerHealthy(workerStaleAfter) {
		status, code = "worker_unhealthy", http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}
