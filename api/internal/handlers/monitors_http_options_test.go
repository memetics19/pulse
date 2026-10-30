package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type monitorAPI struct {
	t      *testing.T
	q      *generated.Queries
	router http.Handler
}

func newMonitorAPI(t *testing.T) *monitorAPI {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)
	r := chi.NewRouter()
	r.Get("/api/monitors", h.List)
	r.Post("/api/monitors", h.Create)
	r.Get("/api/monitors/{id}", h.Get)
	r.Put("/api/monitors/{id}", h.Update)
	return &monitorAPI{t: t, q: q, router: r}
}

func (a *monitorAPI) do(method, path string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(a.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	return rec
}

func httpMonitorBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"name": "api", "url": "https://api.example.com/health", "type": "http",
		"interval_seconds": 60,
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func TestMonitorHTTPOptionsRoundTripWithoutSecret(t *testing.T) {
	a := newMonitorAPI(t)
	rec := a.do(http.MethodPost, "/api/monitors", httpMonitorBody(map[string]any{
		"method":            "post",
		"headers":           map[string]string{"x-api-key": "k1", "Content-Type": "application/json"},
		"body":              `{"ping":true}`,
		"auth":              map[string]string{"type": "basic", "username": "admin", "password": "hunter2-secret"},
		"tls_verify":        false,
		"follow_redirects":  false,
		"accepted_statuses": []string{"200-299", " 301 "},
		"keyword_check":     "error",
		"keyword_mode":      "not_contains",
	}))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "hunter2-secret")

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "POST", got["method"])
	assert.Equal(t, map[string]any{"X-Api-Key": "k1", "Content-Type": "application/json"}, got["headers"])
	assert.Equal(t, `{"ping":true}`, got["body"])
	assert.Equal(t, map[string]any{"type": "basic", "username": "admin", "secret_set": true}, got["auth"])
	assert.Equal(t, false, got["tls_verify"])
	assert.Equal(t, false, got["follow_redirects"])
	assert.Equal(t, []any{"200-299", "301"}, got["accepted_statuses"])
	assert.Equal(t, "not_contains", got["keyword_mode"])
	for _, hidden := range []string{"auth_secret", "auth_type", "auth_username", "password"} {
		assert.NotContains(t, got, hidden)
	}

	id := int64(got["id"].(float64))
	stored, err := a.q.GetMonitor(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "hunter2-secret", stored.AuthSecret)
	assert.Equal(t, "200-299,301", stored.AcceptedStatuses)

	for _, path := range []string{"/api/monitors", fmt.Sprintf("/api/monitors/%d", id)} {
		rec := a.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), "hunter2-secret", path)
		assert.Contains(t, rec.Body.String(), `"secret_set":true`, path)
	}
}

func TestMonitorHTTPOptionsDefaults(t *testing.T) {
	a := newMonitorAPI(t)
	rec := a.do(http.MethodPost, "/api/monitors", httpMonitorBody(nil))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "GET", got["method"])
	assert.Equal(t, map[string]any{}, got["headers"])
	assert.Equal(t, []any{}, got["accepted_statuses"])
	assert.Nil(t, got["auth"])
	assert.Equal(t, true, got["tls_verify"])
	assert.Equal(t, true, got["follow_redirects"])
	assert.Equal(t, "contains", got["keyword_mode"])
}

func TestMonitorHTTPOptionsDefaultsForRowsWithoutThem(t *testing.T) {
	a := newMonitorAPI(t)
	// A row written without the new columns set, like one from before the
	// migration or from a direct insert, reads as the defaults.
	m, err := a.q.CreateMonitor(t.Context(), generated.CreateMonitorParams{
		Name: "old", Url: "https://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 30, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Headers: "not json",
	})
	require.NoError(t, err)
	rec := a.do(http.MethodGet, fmt.Sprintf("/api/monitors/%d", m.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "GET", got["method"])
	assert.Equal(t, "contains", got["keyword_mode"])
	assert.Equal(t, map[string]any{}, got["headers"])
}

func TestMonitorUpdateKeepsWriteOnlySecret(t *testing.T) {
	a := newMonitorAPI(t)
	rec := a.do(http.MethodPost, "/api/monitors", httpMonitorBody(map[string]any{
		"auth": map[string]string{"type": "bearer", "token": "tok-1"},
	}))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct{ ID int64 }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	path := fmt.Sprintf("/api/monitors/%d", created.ID)
	secret := func() (string, string) {
		m, err := a.q.GetMonitor(t.Context(), created.ID)
		require.NoError(t, err)
		return m.AuthType, m.AuthSecret
	}

	// No auth object at all keeps the stored auth.
	rec = a.do(http.MethodPut, path, httpMonitorBody(map[string]any{"name": "renamed"}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	typ, s := secret()
	assert.Equal(t, "bearer", typ)
	assert.Equal(t, "tok-1", s)

	// The same type without a token keeps the token. This is what the admin
	// form sends back after a GET.
	rec = a.do(http.MethodPut, path, httpMonitorBody(map[string]any{
		"auth": map[string]any{"type": "bearer", "secret_set": true},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, s = secret()
	assert.Equal(t, "tok-1", s)

	// A new token replaces it.
	rec = a.do(http.MethodPut, path, httpMonitorBody(map[string]any{
		"auth": map[string]string{"type": "bearer", "token": "tok-2"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, s = secret()
	assert.Equal(t, "tok-2", s)

	// Switching type needs a new secret. The bearer token is not reused as a
	// basic password.
	rec = a.do(http.MethodPut, path, httpMonitorBody(map[string]any{
		"auth": map[string]string{"type": "basic", "username": "u"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	typ, s = secret()
	assert.Equal(t, "basic", typ)
	assert.Empty(t, s)
	rec = a.do(http.MethodPut, path, httpMonitorBody(map[string]any{
		"auth": map[string]string{"type": "bearer"},
	}))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "needs a token")

	// Type none removes auth.
	rec = a.do(http.MethodPut, path, httpMonitorBody(map[string]any{
		"auth": map[string]string{"type": "none"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	typ, s = secret()
	assert.Empty(t, typ)
	assert.Empty(t, s)
	assert.Contains(t, rec.Body.String(), `"auth":null`)
}

func TestMonitorHTTPOptionsValidation(t *testing.T) {
	a := newMonitorAPI(t)
	cases := map[string]map[string]any{
		"method must be GET, HEAD or POST":  {"method": "DELETE"},
		`header "Host" cannot be set`:       {"headers": map[string]string{"Host": "metadata.internal"}},
		"with auth, not headers":            {"headers": map[string]string{"Authorization": "Bearer x"}},
		"control character":                 {"headers": map[string]string{"X-A": "a\r\nb"}},
		"needs a token":                     {"auth": map[string]string{"type": "bearer"}},
		"auth type must be basic or bearer": {"auth": map[string]string{"type": "ntlm"}},
		"from 100 to 599":                   {"accepted_statuses": []string{"2xx"}},
		"no valid PEM certificate":          {"ca_pem": "garbage"},
		"contains or not_contains":          {"keyword_mode": "regex"},
		"body is only sent with POST":       {"body": "x"},
	}
	for want, extra := range cases {
		rec := a.do(http.MethodPost, "/api/monitors", httpMonitorBody(extra))
		assert.Equal(t, http.StatusBadRequest, rec.Code, want)
		assert.Contains(t, rec.Body.String(), want)
	}
}

func TestMonitorHTTPOptionsIgnoredForOtherTypes(t *testing.T) {
	a := newMonitorAPI(t)
	rec := a.do(http.MethodPost, "/api/monitors", map[string]any{
		"name": "db", "url": "db.example.com:5432", "type": "tcp", "interval_seconds": 60,
		"method": "DELETE", "headers": map[string]string{"Host": "x"},
		"auth": map[string]string{"type": "bearer", "token": "t"}, "tls_verify": false,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct{ ID int64 }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	stored, err := a.q.GetMonitor(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "GET", stored.Method)
	assert.Empty(t, stored.Headers)
	assert.Empty(t, stored.AuthSecret)
	assert.Nil(t, stored.TlsVerify)
}
