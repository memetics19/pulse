package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/monitorvalidation"
	pushauth "github.com/memetics19/pulse/api/internal/push"
)

type Monitors struct {
	q            *generated.Queries
	db           func() *sql.DB
	allowPrivate bool
}

func NewMonitors(q *generated.Queries, db func() *sql.DB, allowPrivate bool) *Monitors {
	return &Monitors{q: q, db: db, allowPrivate: allowPrivate}
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
	var request struct {
		generated.CreateMonitorParams
		IsActive *bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := request.CreateMonitorParams
	params.IsActive = true
	if request.IsActive != nil {
		params.IsActive = *request.IsActive
	}
	if reason := monitorvalidation.Validate(monitorvalidation.Input{
		URL: params.Url, Type: params.Type, IntervalSeconds: params.IntervalSeconds,
	}, m.allowPrivate); reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	applyMonitorDefaults(&params.TimeoutSeconds, &params.DegradedThresholdMs, &params.DownThresholdMs)
	if params.Type == "push" {
		m.createPush(w, r, params)
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

type monitorCreateResponse struct {
	generated.Monitor
	PushToken string `json:"push_token,omitempty"`
	PushURL   string `json:"push_url,omitempty"`
}

func (m *Monitors) createPush(w http.ResponseWriter, r *http.Request, params generated.CreateMonitorParams) {
	if m.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	db := m.db()
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create monitor"})
		return
	}
	defer tx.Rollback()
	q := generated.New(tx)
	monitor, err := q.CreateMonitor(r.Context(), params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create monitor"})
		return
	}
	token, err := pushauth.GenerateToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create push credential"})
		return
	}
	if _, err := q.UpsertPushToken(r.Context(), generated.UpsertPushTokenParams{
		MonitorID: monitor.ID, TokenHash: pushauth.HashToken(token), Prefix: pushauth.Prefix(token),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create push credential"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create monitor"})
		return
	}
	writeJSON(w, http.StatusCreated, monitorCreateResponse{
		Monitor: monitor, PushToken: token, PushURL: pushURL(r, token),
	})
}

func (m *Monitors) RotatePushToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	monitor, err := m.q.GetMonitor(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "monitor not found"})
		return
	}
	if monitor.Type != "push" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "monitor is not a push monitor"})
		return
	}
	token, err := pushauth.GenerateToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not rotate push credential"})
		return
	}
	if _, err := m.q.UpsertPushToken(r.Context(), generated.UpsertPushTokenParams{
		MonitorID: id, TokenHash: pushauth.HashToken(token), Prefix: pushauth.Prefix(token),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not rotate push credential"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Token   string `json:"token"`
		PushURL string `json:"push_url"`
	}{
		Token: token, PushURL: pushURL(r, token),
	})
}

func pushURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	forwarded := r.Header.Values("X-Forwarded-Proto")
	if len(forwarded) == 1 && (forwarded[0] == "http" || forwarded[0] == "https") {
		scheme = forwarded[0]
	}
	return (&url.URL{
		Scheme: scheme, Host: r.Host, Path: "/api/push/" + token,
		RawQuery: "status=up&msg=OK&ping=",
	}).String()
}

func (m *Monitors) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	existing, err := m.q.GetMonitor(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "monitor not found"})
		return
	}
	var params generated.UpdateMonitorParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if existing.Type != params.Type && (existing.Type == "push" || params.Type == "push") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "push monitor type cannot be changed; delete and recreate the monitor",
		})
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
