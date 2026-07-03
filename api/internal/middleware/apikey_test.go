package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
	"github.com/memetics19/pulse/api/testutil"
)

func TestRequireSessionOrAPIKey(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := RequireSessionOrAPIKey(q)(ok)

	// (a) no credentials -> 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/monitors", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no cred=%d want 401", rec.Code)
	}

	// (b) valid session -> 200
	hash, _ := auth.HashPassword("pw-at-least-8")
	u, _ := q.CreateUser(t.Context(), generated.CreateUserParams{Username: "a", PasswordHash: hash})
	tok, _ := auth.NewSessionToken()
	_ = q.CreateSession(t.Context(), generated.CreateSessionParams{Token: tok, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	req := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: tok})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("session=%d want 200", rec.Code)
	}

	// API key with monitors:read
	full, prefix, kh, _ := keyauth.Generate()
	sc, _ := json.Marshal([]string{"monitors:read"})
	_, _ = q.CreateAPIKey(t.Context(), generated.CreateAPIKeyParams{Name: "k", KeyHash: kh, Prefix: prefix, Scopes: string(sc)})

	// (c) GET /api/monitors with the key -> 200
	req = httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	req.Header.Set("Authorization", "Bearer "+full)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("key read=%d want 200", rec.Code)
	}

	// (d) POST /api/monitors with the same key (needs monitors:write) -> 403
	req = httptest.NewRequest(http.MethodPost, "/api/monitors", nil)
	req.Header.Set("Authorization", "Bearer "+full)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("key write=%d want 403", rec.Code)
	}

	// (e) unknown/garbage key -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	req.Header.Set("Authorization", "Bearer pulse_live_garbage")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad key=%d want 401", rec.Code)
	}
}

func TestHasScopeExactMatchOnly(t *testing.T) {
	cases := []struct {
		scopes string
		need   string
		want   bool
	}{
		{`["monitors:read"]`, "monitors:read", true},
		{`["monitors:readonly"]`, "monitors:read", false}, // substring must not match
		{`["monitors:read","incidents:write"]`, "incidents:write", true},
		{`not json`, "monitors:read", false},
		{`[]`, "monitors:read", false},
	}
	for _, c := range cases {
		if got := hasScope(c.scopes, c.need); got != c.want {
			t.Errorf("hasScope(%q, %q) = %v, want %v", c.scopes, c.need, got, c.want)
		}
	}
}
