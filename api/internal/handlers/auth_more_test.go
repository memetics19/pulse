package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

// setupAdmin creates the admin account and returns the session cookie.
func setupAdmin(t *testing.T, h *Auth) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})
	rec := httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie from setup")
	return nil
}

func TestTwoFAErrorBranches(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	h := NewAuth(q, false)
	cookie := setupAdmin(t, h)

	req := func(withCookie bool, body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(body)))
		if withCookie {
			r.AddCookie(cookie)
		}
		return r
	}
	code := func(hf func(http.ResponseWriter, *http.Request), r *http.Request) int {
		rec := httptest.NewRecorder()
		hf(rec, r)
		return rec.Code
	}

	// Unauthenticated -> 401
	if c := code(h.TwoFASetup, req(false, "")); c != http.StatusUnauthorized {
		t.Fatalf("TwoFASetup no session = %d, want 401", c)
	}
	if c := code(h.TwoFAEnable, req(false, `{}`)); c != http.StatusUnauthorized {
		t.Fatalf("TwoFAEnable no session = %d, want 401", c)
	}
	// Enable before setup -> "run setup first" 400
	if c := code(h.TwoFAEnable, req(true, `{"code":"000000"}`)); c != http.StatusBadRequest {
		t.Fatalf("TwoFAEnable before setup = %d, want 400", c)
	}
	// Setup stores a pending secret -> 200
	if c := code(h.TwoFASetup, req(true, "")); c != http.StatusOK {
		t.Fatalf("TwoFASetup = %d, want 200", c)
	}
	// Enable with a wrong code -> 400
	if c := code(h.TwoFAEnable, req(true, `{"code":"000000"}`)); c != http.StatusBadRequest {
		t.Fatalf("TwoFAEnable wrong code = %d, want 400", c)
	}
}

func TestAuthSessionMethods(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	h := NewAuth(q, false)
	cookie := setupAdmin(t, h)

	withCookie := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(cookie)
		return r
	}

	// Status with a valid session -> authenticated
	rec := httptest.NewRecorder()
	h.Status(rec, withCookie(http.MethodGet, "/api/auth/status"))
	var st struct {
		Authenticated bool `json:"authenticated"`
		Username      string
	}
	json.NewDecoder(rec.Body).Decode(&st)
	if !st.Authenticated || st.Username != "admin" {
		t.Fatalf("authenticated status wrong: %+v", st)
	}

	// TwoFADisable without a session -> 401
	rec = httptest.NewRecorder()
	h.TwoFADisable(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("2FA disable unauthenticated = %d, want 401", rec.Code)
	}

	// TwoFADisable with a session -> 200
	rec = httptest.NewRecorder()
	h.TwoFADisable(rec, withCookie(http.MethodPost, "/x"))
	if rec.Code != http.StatusOK {
		t.Fatalf("2FA disable = %d, want 200", rec.Code)
	}

	// Logout clears the session
	rec = httptest.NewRecorder()
	h.Logout(rec, withCookie(http.MethodPost, "/api/auth/logout"))
	if rec.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", rec.Code)
	}

	// Login with malformed body -> 400
	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte("not json"))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad login body = %d, want 400", rec.Code)
	}
}
