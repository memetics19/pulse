package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

// Defaults applied when a monitor is created or updated without a given field
// (for example by the Uptime Kuma importer, which omits the timeout). They
// mirror the schema column defaults, which are otherwise bypassed because the
// generated params struct always sends an explicit value in the INSERT.
const (
	defaultTimeoutSeconds  = 30
	defaultDegradedMs      = 500
	defaultDownMs          = 2000
	defaultTLSWarnDays     = 14
	defaultTLSCriticalDays = 7

	defaultFailuresBeforeIncident  = 2
	defaultRecoveriesBeforeResolve = 2
	defaultRetries                 = 1
	defaultRetryIntervalMs         = 500
)

// Allowed ranges for the sensitivity settings. The retry limits keep a single
// check from running for minutes when a target times out.
const (
	minSensitivityCount = 1
	maxSensitivityCount = 10
	maxRetries          = 5
	maxRetryIntervalMs  = 60000
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
	TLSWarnDays         *int64 `json:"tls_warn_days"`
	TLSCriticalDays     *int64 `json:"tls_critical_days"`
	IsActive            *bool  `json:"is_active"`
	GroupID             *int64 `json:"group_id"`
	Source              string `json:"source"`
	ExternalID          string `json:"external_id"`

	FailuresBeforeIncident  *int64 `json:"failures_before_incident"`
	RecoveriesBeforeResolve *int64 `json:"recoveries_before_resolve"`
	Retries                 *int64 `json:"retries"`
	RetryIntervalMs         *int64 `json:"retry_interval_ms"`
}

// resolved holds the request's fields with defaults filled in. It is the single
// place defaults and cross-field rules (degraded < down, TLS warn > critical)
// are applied, shared by Create and Update.
type resolvedMonitor struct {
	req                      monitorRequest
	timeout, degraded, down  int64
	failures, recoveries     int64
	retries, retryIntervalMs int64
	tlsWarn, tlsCritical     int64
	isActive                 bool
}

// resolve validates the request and fills defaults. reason is non-empty on a
// validation failure. Target validation (including the SSRF guard for every
// monitor type) lives in monitorvalidation so the importer shares it.
func (m *Monitors) resolve(req monitorRequest) (resolvedMonitor, string) {
	if reason := monitorvalidation.Validate(monitorvalidation.Input{
		URL: req.Url, Type: req.Type, IntervalSeconds: req.IntervalSeconds,
	}, m.allowPrivate); reason != "" {
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
	var reason string
	if r.failures, reason = inRange("failures_before_incident", req.FailuresBeforeIncident,
		defaultFailuresBeforeIncident, minSensitivityCount, maxSensitivityCount); reason != "" {
		return resolvedMonitor{}, reason
	}
	if r.recoveries, reason = inRange("recoveries_before_resolve", req.RecoveriesBeforeResolve,
		defaultRecoveriesBeforeResolve, minSensitivityCount, maxSensitivityCount); reason != "" {
		return resolvedMonitor{}, reason
	}
	if r.retries, reason = inRange("retries", req.Retries, defaultRetries, 0, maxRetries); reason != "" {
		return resolvedMonitor{}, reason
	}
	if r.retryIntervalMs, reason = inRange("retry_interval_ms", req.RetryIntervalMs,
		defaultRetryIntervalMs, 0, maxRetryIntervalMs); reason != "" {
		return resolvedMonitor{}, reason
	}
	// Unlike the latency thresholds, 0 is a valid TLS critical threshold
	// (down only on the last day), so only nil takes the default.
	r.tlsWarn = defaultTLSWarnDays
	if req.TLSWarnDays != nil {
		r.tlsWarn = *req.TLSWarnDays
	}
	r.tlsCritical = defaultTLSCriticalDays
	if req.TLSCriticalDays != nil {
		r.tlsCritical = *req.TLSCriticalDays
	}
	if r.tlsCritical < 0 {
		return resolvedMonitor{}, "tls_critical_days must be 0 or more"
	}
	if r.tlsWarn <= r.tlsCritical {
		return resolvedMonitor{}, "tls_warn_days must be greater than tls_critical_days"
	}
	return r, ""
}

// inRange returns def when v is omitted, or v when it lies in [lo, hi]. reason
// is non-empty when v is out of range. Unlike the latency thresholds, an
// explicit 0 is not read as "use the default", since 0 retries is a real
// setting.
func inRange(name string, v *int64, def, lo, hi int64) (int64, string) {
	if v == nil {
		return def, ""
	}
	if *v < lo || *v > hi {
		return 0, fmt.Sprintf("%s must be between %d and %d", name, lo, hi)
	}
	return *v, ""
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
	encodeJSON(w, monitors)
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
	encodeJSON(w, monitor)
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

		FailuresBeforeIncident:  res.failures,
		RecoveriesBeforeResolve: res.recoveries,
		Retries:                 res.retries,
		RetryIntervalMs:         res.retryIntervalMs,
		TlsWarnDays:             res.tlsWarn,
		TlsCriticalDays:         res.tlsCritical,
	}
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
	encodeJSON(w, monitor)
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
	// Rollback is a no-op once Commit succeeds; on failure the response
	// already reports the original error.
	defer func() { _ = tx.Rollback() }()
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
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "monitor not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load monitor"})
		return
	}
	var req monitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if existing.Type != req.Type && (existing.Type == "push" || req.Type == "push") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "push monitor type cannot be changed; delete and recreate the monitor",
		})
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

		FailuresBeforeIncident:  res.failures,
		RecoveriesBeforeResolve: res.recoveries,
		Retries:                 res.retries,
		RetryIntervalMs:         res.retryIntervalMs,
		TlsWarnDays:             res.tlsWarn,
		TlsCriticalDays:         res.tlsCritical,
		ID:                      id,
	}
	monitor, err := m.q.UpdateMonitor(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, monitor)
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
