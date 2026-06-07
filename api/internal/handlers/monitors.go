package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
)

type Monitors struct {
	q *generated.Queries
}

func NewMonitors(q *generated.Queries) *Monitors {
	return &Monitors{q: q}
}

// validMonitorTypes is the set of monitor types the scheduler can check.
var validMonitorTypes = map[string]bool{
	"http": true, "https": true, "tcp": true, "ping": true,
	"dns": true, "ssl": true, "infra": true,
}

// validateMonitorInput returns a human-readable reason when any required field
// is missing or out of range, or "" when the input is acceptable.
func validateMonitorInput(url, monType string, intervalSeconds, timeoutSeconds int64) string {
	switch {
	case url == "":
		return "url is required"
	case !validMonitorTypes[monType]:
		return "invalid type"
	case intervalSeconds < 1:
		return "interval_seconds must be at least 1"
	case timeoutSeconds < 1:
		return "timeout_seconds must be at least 1"
	}
	return ""
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
	if reason := validateMonitorInput(params.Url, params.Type, params.IntervalSeconds, params.TimeoutSeconds); reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
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
	if reason := validateMonitorInput(params.Url, params.Type, params.IntervalSeconds, params.TimeoutSeconds); reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
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
