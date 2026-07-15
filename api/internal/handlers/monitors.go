package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/netguard"
)

type Monitors struct {
	q            *generated.Queries
	allowPrivate bool
}

func NewMonitors(q *generated.Queries, allowPrivate bool) *Monitors {
	return &Monitors{q: q, allowPrivate: allowPrivate}
}

// validMonitorTypes is the set of monitor types the scheduler can check. Note
// there is no "https": the "http" checker handles https URLs, and the schema's
// CHECK constraint forbids "https", so accepting it here would 500 at insert.
var validMonitorTypes = map[string]bool{
	"http": true, "tcp": true, "ping": true,
	"dns": true, "ssl": true, "infra": true,
}

// Defaults applied when a monitor is created or updated without a given field.
// They mirror the schema column defaults, which are otherwise bypassed because
// the generated params struct always sends an explicit value in the INSERT.
const (
	defaultTimeoutSeconds = 30
	defaultDegradedMs     = 500
	defaultDownMs         = 2000
)

// monitorRequest is the decode target for Create/Update. Pointer fields let the
// handler distinguish "omitted" (nil → apply default) from "explicitly zero",
// so an omitted is_active defaults to true (scheduled) and omitted thresholds
// get the schema defaults instead of 0 (which would flap every check to down).
type monitorRequest struct {
	Name                string `json:"name"`
	Url                 string `json:"url"`
	Type                string `json:"type"`
	IntervalSeconds     int64  `json:"interval_seconds"`
	TimeoutSeconds      *int64 `json:"timeout_seconds"`
	ExpectedStatus      *int64 `json:"expected_status"`
	KeywordCheck        string `json:"keyword_check"`
	DegradedThresholdMs *int64 `json:"degraded_threshold_ms"`
	DownThresholdMs     *int64 `json:"down_threshold_ms"`
	IsActive            *bool  `json:"is_active"`
	GroupID             *int64 `json:"group_id"`
	Source              string `json:"source"`
	ExternalID          string `json:"external_id"`
}

// resolved holds the request's fields with defaults filled in. It is the single
// place defaults and cross-field rules (degraded < down) are applied, shared by
// Create and Update.
type resolvedMonitor struct {
	req                     monitorRequest
	timeout, degraded, down int64
	isActive                bool
}

// resolve validates the request and fills defaults. reason is non-empty on a
// validation failure.
func (m *Monitors) resolve(req monitorRequest) (resolvedMonitor, string) {
	if reason := m.validateMonitorInput(req.Url, req.Type, req.IntervalSeconds); reason != "" {
		return resolvedMonitor{}, reason
	}
	r := resolvedMonitor{req: req, isActive: true}
	if req.IsActive != nil {
		r.isActive = *req.IsActive
	}
	r.timeout = defaultTimeoutSeconds
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds > 0 {
		r.timeout = *req.TimeoutSeconds
	}
	r.degraded = defaultDegradedMs
	if req.DegradedThresholdMs != nil && *req.DegradedThresholdMs > 0 {
		r.degraded = *req.DegradedThresholdMs
	}
	r.down = defaultDownMs
	if req.DownThresholdMs != nil && *req.DownThresholdMs > 0 {
		r.down = *req.DownThresholdMs
	}
	if r.degraded >= r.down {
		return resolvedMonitor{}, "degraded_threshold_ms must be less than down_threshold_ms"
	}
	return r, ""
}

// validateMonitorInput returns a human-readable reason when a required field is
// missing or out of range, or "" when the input is acceptable. A zero interval
// is rejected because it would panic the scheduler's ticker; a missing timeout
// is defaulted by the caller rather than rejected.
func (m *Monitors) validateMonitorInput(url, monType string, intervalSeconds int64) string {
	switch {
	case url == "":
		return "url is required"
	case !validMonitorTypes[monType]:
		return "invalid type"
	case intervalSeconds < 1:
		return "interval_seconds must be at least 1"
	}
	// Every monitor type dials or resolves a user-supplied target, so reject
	// targets pointing at private/internal networks unless explicitly allowed
	// (SSRF guard). HTTP(S) targets are URLs; the rest are host[:port]/hostnames.
	if monType == "http" || monType == "https" {
		if err := netguard.ValidateURL(url, m.allowPrivate); err != nil {
			return err.Error()
		}
	} else {
		if err := netguard.ValidateTarget(url, m.allowPrivate); err != nil {
			return err.Error()
		}
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
	var req monitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, reason := m.resolve(req)
	if reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	params := generated.CreateMonitorParams{
		Name:                req.Name,
		Url:                 req.Url,
		Type:                req.Type,
		IntervalSeconds:     req.IntervalSeconds,
		TimeoutSeconds:      res.timeout,
		ExpectedStatus:      req.ExpectedStatus,
		KeywordCheck:        req.KeywordCheck,
		DegradedThresholdMs: res.degraded,
		DownThresholdMs:     res.down,
		IsActive:            res.isActive,
		GroupID:             req.GroupID,
		Source:              req.Source,
		ExternalID:          req.ExternalID,
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
	var req monitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, reason := m.resolve(req)
	if reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	params := generated.UpdateMonitorParams{
		Name:                req.Name,
		Url:                 req.Url,
		Type:                req.Type,
		IntervalSeconds:     req.IntervalSeconds,
		TimeoutSeconds:      res.timeout,
		ExpectedStatus:      req.ExpectedStatus,
		KeywordCheck:        req.KeywordCheck,
		DegradedThresholdMs: res.degraded,
		DownThresholdMs:     res.down,
		IsActive:            res.isActive,
		GroupID:             req.GroupID,
		ID:                  id,
	}
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
