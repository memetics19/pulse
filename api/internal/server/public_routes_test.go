package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// leakMarkers are values that must never reach an anonymous caller: the
// monitor target, the raw dialer error, the raw check fields, and the author
// of an incident update.
var leakMarkers = []string{
	"10.0.3.7", "internal-target", "connection refused",
	"error_message", "status_code", "alice-oncall",
}

type leakFixture struct {
	h          http.Handler
	q          *generated.Queries
	monitorID  int64
	incidentID int64
}

func newLeakFixture(t *testing.T) leakFixture {
	t.Helper()
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	ctx := t.Context()

	g, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "Core"})
	require.NoError(t, err)
	m, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "Primary DB", Url: "tcp://10.0.3.7:5432/internal-target", Type: "tcp",
		IntervalSeconds: 60, TimeoutSeconds: 10, DegradedThresholdMs: 500,
		DownThresholdMs: 2000, IsActive: true, GroupID: &g.ID, Source: "internal",
	})
	require.NoError(t, err)
	code := int64(503)
	ms := int64(12)
	for i, st := range []string{"up", "down"} {
		_, err = q.InsertCheckResult(ctx, generated.InsertCheckResultParams{
			MonitorID: m.ID, CheckedAt: time.Now().Add(-time.Duration(2-i) * time.Minute),
			Status: st, ResponseTimeMs: &ms, StatusCode: &code,
			ErrorMessage: "dial tcp 10.0.3.7:5432: connect: connection refused",
		})
		require.NoError(t, err)
	}
	inc, err := q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "Primary DB is down", Severity: "major",
		AffectedMonitorIds: fmt.Sprintf("[%d]", m.ID), StartedAt: time.Now(), Source: "internal",
	})
	require.NoError(t, err)
	_, err = q.CreateIncidentUpdate(ctx, generated.CreateIncidentUpdateParams{
		IncidentID: inc.ID, Status: "investigating", Message: "We are looking into it.", Author: "alice-oncall",
	})
	require.NoError(t, err)

	a := app.New()
	a.SetDB(db)
	a.MarkWorkerAlive()
	return leakFixture{
		h: server.New(a, t.TempDir(), config.Config{}), q: q,
		monitorID: m.ID, incidentID: inc.ID,
	}
}

func (f leakFixture) serve(method, path string, mod func(*http.Request)) *httptest.ResponseRecorder {
	var body *strings.Reader
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		body = strings.NewReader("{}")
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

// publicRoutes lists every route an anonymous caller may reach. Method "*"
// covers routes mounted with Handle, which answer every method. Any route
// missing here must refuse an anonymous request with 401, so a new route
// cannot become public by accident.
var publicRoutes = map[string]bool{
	"GET /api/setup/state":         true,
	"POST /api/setup":              true,
	"GET /":                        true,
	"GET /feed.xml":                true,
	"* /static/*":                  true,
	"GET /healthz":                 true,
	"* /metrics":                   true,
	"GET /api/status":              true,
	"POST /api/ingest/metrics":     true,
	"POST /api/ingest/diagnostics": true,
	"GET /api/push/{token}":        true,
	"POST /api/push/{token}":       true,
	"POST /api/auth/login":         true,
	"POST /api/auth/logout":        true,
	"GET /api/auth/status":         true,
	"* /admin":                     true,
	"* /admin/*":                   true,
	"* /setup":                     true,
	"* /setup/*":                   true,
	"* /_next/*":                   true,
	"GET /favicon.ico":             true,
}

func isPublic(method, route string) bool {
	return publicRoutes[method+" "+route] || publicRoutes["* "+route]
}

func fillRoute(route string, f leakFixture) string {
	r := strings.NewReplacer(
		"{monitorID}", strconv.FormatInt(f.monitorID, 10),
		"{incidentID}", strconv.FormatInt(f.incidentID, 10),
		"{id}", strconv.FormatInt(f.monitorID, 10),
		"{agentID}", "1",
		"{token}", "not-a-token",
		"*", "",
	)
	return r.Replace(route)
}

// Walk every registered route as an anonymous caller. Public routes may answer
// but must not carry any target, raw error, raw check field or update author.
// Every other route must answer 401.
func TestAnonymousRoutesLeakNothing(t *testing.T) {
	t.Setenv("PULSE_METRICS", "true")
	t.Setenv("PULSE_METRICS_TOKEN", "")
	f := newLeakFixture(t)

	routes, ok := f.h.(chi.Routes)
	require.True(t, ok, "server.New must return a chi router")

	seen := 0
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen++
		path := fillRoute(route, f)
		rec := f.serve(method, path, nil)
		body := rec.Body.String()
		for _, m := range leakMarkers {
			assert.NotContains(t, body, m, "%s %s leaks %q to an anonymous caller", method, route, m)
		}
		if !isPublic(method, route) {
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s must refuse anonymous callers", method, route)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Greater(t, seen, 40, "the walk should cover the whole router")
}

// The issue's exact probe: the three routes that used to be public now refuse
// anonymous callers, for real and made-up IDs alike.
func TestCheckAndUpdateRoutesRefuseAnonymous(t *testing.T) {
	f := newLeakFixture(t)
	for _, p := range []string{
		fmt.Sprintf("/api/monitors/%d/checks?days=90", f.monitorID),
		fmt.Sprintf("/api/monitors/%d/checks/uptime?days=100000", f.monitorID),
		fmt.Sprintf("/api/incidents/%d/updates", f.incidentID),
		"/api/monitors/999/checks",
		"/api/incidents/999/updates",
	} {
		rec := f.serve(http.MethodGet, p, nil)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, p)
		assert.Equal(t, "unauthorized\n", rec.Body.String(), p)
	}
}

// Each route needs the read scope of its own resource. A session still gets
// the full rows the admin UI shows.
func TestCheckAndUpdateRoutesEnforceScopes(t *testing.T) {
	f := newLeakFixture(t)
	checks := fmt.Sprintf("/api/monitors/%d/checks?days=7", f.monitorID)
	uptime := fmt.Sprintf("/api/monitors/%d/checks/uptime", f.monitorID)
	updates := fmt.Sprintf("/api/incidents/%d/updates", f.incidentID)

	cases := []struct {
		path, scope string
		want        int
	}{
		{checks, "monitors:read", http.StatusOK},
		{uptime, "monitors:read", http.StatusOK},
		{updates, "incidents:read", http.StatusOK},
		{checks, "incidents:read", http.StatusForbidden},
		{uptime, "status:read", http.StatusForbidden},
		{checks, "monitors:write", http.StatusForbidden},
		{updates, "monitors:read", http.StatusForbidden},
		{updates, "incidents:write", http.StatusForbidden},
	}
	for _, c := range cases {
		key := createServerAPIKey(t, f.q, c.scope)
		rec := f.serve(http.MethodGet, c.path, func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+key)
		})
		assert.Equal(t, c.want, rec.Code, "%s with a %s key", c.path, c.scope)
	}

	bad := f.serve(http.MethodGet, checks, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer pulse_live_revoked_or_made_up")
	})
	assert.Equal(t, http.StatusUnauthorized, bad.Code)

	cookie := createServerSession(t, f.q)
	withSession := func(r *http.Request) { r.AddCookie(cookie) }

	rec := f.serve(http.MethodGet, checks, withSession)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "connection refused", "admins still see the raw error")

	rec = f.serve(http.MethodGet, uptime, withSession)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"uptime_pct":50`)

	rec = f.serve(http.MethodGet, updates, withSession)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "alice-oncall")
}

// The public page, its JSON and its feed keep working without the routes that
// moved behind auth: they render from the server, not from client fetches.
func TestPublicPageStillRenders(t *testing.T) {
	f := newLeakFixture(t)

	page := f.serve(http.MethodGet, "/", nil)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Primary DB")
	assert.Contains(t, body, "Primary DB is down")
	assert.Contains(t, body, "We are looking into it.")
	assert.Contains(t, body, "50.00%", "uptime comes from the server-side series")

	status := f.serve(http.MethodGet, "/api/status", nil)
	require.Equal(t, http.StatusOK, status.Code)
	assert.Contains(t, status.Body.String(), `{"name":"Primary DB","status":"down"}`)

	feed := f.serve(http.MethodGet, "/feed.xml", nil)
	require.Equal(t, http.StatusOK, feed.Code)
	assert.Contains(t, feed.Body.String(), "We are looking into it.")
}

func createServerSession(t *testing.T, q *generated.Queries) *http.Cookie {
	t.Helper()
	u, err := q.CreateUser(t.Context(), generated.CreateUserParams{Username: "admin", PasswordHash: "x"})
	require.NoError(t, err)
	token := "session-token-for-tests"
	require.NoError(t, q.CreateSession(t.Context(), generated.CreateSessionParams{
		Token: token, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour),
	}))
	return &http.Cookie{Name: auth.SessionCookieName, Value: token}
}
