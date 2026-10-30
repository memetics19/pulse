package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

	// HTTP options. They apply to http and https monitors only and are
	// stored as their defaults for every other type.
	Method           string              `json:"method"`
	Headers          map[string]string   `json:"headers"`
	Body             string              `json:"body"`
	Auth             *monitorAuthRequest `json:"auth"`
	TLSVerify        *bool               `json:"tls_verify"`
	CAPEM            string              `json:"ca_pem"`
	FollowRedirects  *bool               `json:"follow_redirects"`
	AcceptedStatuses []string            `json:"accepted_statuses"`
	KeywordMode      string              `json:"keyword_mode"`
}

// monitorAuthRequest sets the monitor's auth. Password is the basic password
// and Token the bearer token. Both are write-only. On an update, an omitted
// auth object keeps the stored auth, and an omitted secret keeps the stored
// secret when the type is unchanged. Type "" or "none" removes auth.
type monitorAuthRequest struct {
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

// monitorAuthView is the auth as the API returns it: never the secret, only
// whether one is stored.
type monitorAuthView struct {
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	SecretSet bool   `json:"secret_set"`
}

// monitorResponse is a monitor as the API returns it. The stored header map
// and status list are decoded, the nullable flags read as plain booleans, and
// the auth secret is left out (its column is tagged json:"-").
type monitorResponse struct {
	generated.Monitor
	Headers          map[string]string `json:"headers"`
	AcceptedStatuses []string          `json:"accepted_statuses"`
	TLSVerify        bool              `json:"tls_verify"`
	FollowRedirects  bool              `json:"follow_redirects"`
	Auth             *monitorAuthView  `json:"auth"`
}

func toMonitorResponse(m generated.Monitor) monitorResponse {
	res := monitorResponse{
		Monitor:          m,
		Headers:          map[string]string{},
		AcceptedStatuses: []string{},
		TLSVerify:        m.TlsVerify == nil || *m.TlsVerify,
		FollowRedirects:  m.FollowRedirects == nil || *m.FollowRedirects,
	}
	if res.Method == "" {
		res.Method = http.MethodGet
	}
	if res.KeywordMode == "" {
		res.KeywordMode = monitorvalidation.KeywordContains
	}
	if m.Headers != "" {
		// A row that does not decode shows no headers. The API only stores
		// what it encoded itself.
		_ = json.Unmarshal([]byte(m.Headers), &res.Headers)
	}
	if m.AcceptedStatuses != "" {
		res.AcceptedStatuses = strings.Split(m.AcceptedStatuses, ",")
	}
	if m.AuthType != "" {
		res.Auth = &monitorAuthView{Type: m.AuthType, Username: m.AuthUsername, SecretSet: m.AuthSecret != ""}
	}
	return res
}

// httpColumns are the stored forms of the HTTP options.
type httpColumns struct {
	method, headers, body                string
	authType, authUsername, authSecret   string
	caPEM, acceptedStatuses, keywordMode string
	tlsVerify, followRedirects           *bool
}

// resolveHTTP validates the HTTP options and returns their stored forms.
// existing is the stored monitor on an update, nil on a create, and is where
// an omitted auth or secret is kept from.
func resolveHTTP(req monitorRequest, existing *generated.Monitor) (httpColumns, string) {
	if req.Type != "http" && req.Type != "https" {
		return httpColumns{method: http.MethodGet, keywordMode: monitorvalidation.KeywordContains}, ""
	}
	cols := httpColumns{
		method:          strings.ToUpper(strings.TrimSpace(req.Method)),
		body:            req.Body,
		caPEM:           strings.TrimSpace(req.CAPEM),
		keywordMode:     req.KeywordMode,
		tlsVerify:       req.TLSVerify,
		followRedirects: req.FollowRedirects,
	}
	if cols.method == "" {
		cols.method = http.MethodGet
	}
	if cols.keywordMode == "" {
		cols.keywordMode = monitorvalidation.KeywordContains
	}
	cols.authType, cols.authUsername, cols.authSecret = resolveAuth(req.Auth, existing)
	if reason := monitorvalidation.ValidateHTTP(monitorvalidation.HTTPOptions{
		Method: cols.method, Headers: req.Headers, Body: cols.body,
		AuthType: cols.authType, AuthUsername: cols.authUsername, AuthSecret: cols.authSecret,
		CAPEM: cols.caPEM, AcceptedStatuses: req.AcceptedStatuses,
		KeywordMode: cols.keywordMode, Keyword: req.KeywordCheck,
	}); reason != "" {
		return httpColumns{}, reason
	}
	if len(req.Headers) > 0 {
		canonical := make(map[string]string, len(req.Headers))
		for name, value := range req.Headers {
			canonical[http.CanonicalHeaderKey(name)] = value
		}
		// A map of strings always encodes.
		b, _ := json.Marshal(canonical)
		cols.headers = string(b)
	}
	// ValidateHTTP already parsed the list, so this cannot fail.
	ranges, _ := monitorvalidation.ParseStatusRanges(req.AcceptedStatuses)
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = r.String()
	}
	cols.acceptedStatuses = strings.Join(parts, ",")
	return cols, ""
}

// resolveAuth applies the write-only rules for the auth secret.
func resolveAuth(req *monitorAuthRequest, existing *generated.Monitor) (authType, username, secret string) {
	if req == nil {
		if existing != nil {
			return existing.AuthType, existing.AuthUsername, existing.AuthSecret
		}
		return "", "", ""
	}
	authType = strings.ToLower(strings.TrimSpace(req.Type))
	switch authType {
	case "", "none":
		return "", "", ""
	case monitorvalidation.AuthBearer:
		secret = req.Token
	default:
		username, secret = req.Username, req.Password
	}
	if secret == "" && existing != nil && existing.AuthType == authType {
		secret = existing.AuthSecret
	}
	return authType, username, secret
}

// resolved holds the request's fields with defaults filled in. It is the single
// place defaults and cross-field rules (degraded < down, TLS warn > critical)
// are applied, shared by Create and Update.
type resolvedMonitor struct {
	req                     monitorRequest
	timeout, degraded, down int64
	tlsWarn, tlsCritical    int64
	isActive                bool
	http                    httpColumns
}

// resolve validates the request and fills defaults. reason is non-empty on a
// validation failure. Target validation (including the SSRF guard for every
// monitor type) lives in monitorvalidation so the importer shares it.
// existing is the stored monitor on an update and nil on a create.
func (m *Monitors) resolve(req monitorRequest, existing *generated.Monitor) (resolvedMonitor, string) {
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
	cols, reason := resolveHTTP(req, existing)
	if reason != "" {
		return resolvedMonitor{}, reason
	}
	r.http = cols
	return r, ""
}

func (m *Monitors) List(w http.ResponseWriter, r *http.Request) {
	monitors, err := m.q.ListMonitors(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]monitorResponse, len(monitors))
	for i, mon := range monitors {
		out[i] = toMonitorResponse(mon)
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, out)
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
	encodeJSON(w, toMonitorResponse(monitor))
}

func (m *Monitors) Create(w http.ResponseWriter, r *http.Request) {
	var req monitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, reason := m.resolve(req, nil)
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
		TlsWarnDays:         res.tlsWarn,
		TlsCriticalDays:     res.tlsCritical,
		Method:              res.http.method,
		Headers:             res.http.headers,
		Body:                res.http.body,
		AuthType:            res.http.authType,
		AuthUsername:        res.http.authUsername,
		AuthSecret:          res.http.authSecret,
		TlsVerify:           res.http.tlsVerify,
		CaPem:               res.http.caPEM,
		FollowRedirects:     res.http.followRedirects,
		AcceptedStatuses:    res.http.acceptedStatuses,
		KeywordMode:         res.http.keywordMode,
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
	encodeJSON(w, toMonitorResponse(monitor))
}

type monitorCreateResponse struct {
	monitorResponse
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
		monitorResponse: toMonitorResponse(monitor), PushToken: token, PushURL: pushURL(r, token),
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
	res, reason := m.resolve(req, &existing)
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
		TlsWarnDays:         res.tlsWarn,
		TlsCriticalDays:     res.tlsCritical,
		Method:              res.http.method,
		Headers:             res.http.headers,
		Body:                res.http.body,
		AuthType:            res.http.authType,
		AuthUsername:        res.http.authUsername,
		AuthSecret:          res.http.authSecret,
		TlsVerify:           res.http.tlsVerify,
		CaPem:               res.http.caPEM,
		FollowRedirects:     res.http.followRedirects,
		AcceptedStatuses:    res.http.acceptedStatuses,
		KeywordMode:         res.http.keywordMode,
		ID:                  id,
	}
	monitor, err := m.q.UpdateMonitor(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, toMonitorResponse(monitor))
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
