package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/monitorvalidation"
)

type Monitors struct {
	q            *generated.Queries
	allowPrivate bool
}

func NewMonitors(q *generated.Queries, allowPrivate bool) *Monitors {
	return &Monitors{q: q, allowPrivate: allowPrivate}
}

// defaultTimeoutSeconds is applied when a monitor is created or updated without
// a timeout (for example by the Uptime Kuma importer, which omits the field).
const defaultTimeoutSeconds = 30
const defaultDegradedThresholdMs = 500
const defaultDownThresholdMs = 2000

func applyMonitorDefaults(timeout, degraded, down *int64) {
	if *timeout < 1 {
		*timeout = defaultTimeoutSeconds
	}
	if *degraded < 1 {
		*degraded = defaultDegradedThresholdMs
	}
	if *down < 1 {
		*down = defaultDownThresholdMs
	}
}

func (m *Monitors) List(w http.ResponseWriter, r *http.Request) {
	monitors, err := m.q.ListMonitors(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if monitors == nil {
		monitors = []generated.Monitor{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(monitors)
}

func (m *Monitors) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	monitor, err := m.q.GetMonitor(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(monitor)
}

func (m *Monitors) Create(w http.ResponseWriter, r *http.Request) {
	var params generated.CreateMonitorParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if reason := monitorvalidation.Validate(monitorvalidation.Input{
		URL: params.Url, Type: params.Type, IntervalSeconds: params.IntervalSeconds,
	}, m.allowPrivate); reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	applyMonitorDefaults(&params.TimeoutSeconds, &params.DegradedThresholdMs, &params.DownThresholdMs)
	monitor, err := m.q.CreateMonitor(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(monitor)
}

func (m *Monitors) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var params generated.UpdateMonitorParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if reason := monitorvalidation.Validate(monitorvalidation.Input{
		URL: params.Url, Type: params.Type, IntervalSeconds: params.IntervalSeconds,
	}, m.allowPrivate); reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	applyMonitorDefaults(&params.TimeoutSeconds, &params.DegradedThresholdMs, &params.DownThresholdMs)
	params.ID = id
	monitor, err := m.q.UpdateMonitor(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(monitor)
}

func (m *Monitors) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := m.q.DeleteMonitor(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
