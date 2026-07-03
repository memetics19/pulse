package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/pquerna/otp/totp"
)

func TestAuthSetupThenLoginThenStatus(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := NewAuth(q, false)

	rec := httptest.NewRecorder()
	h.Status(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	var st struct {
		NeedsSetup    bool `json:"needs_setup"`
		Authenticated bool `json:"authenticated"`
	}
	json.NewDecoder(rec.Body).Decode(&st)
	if !st.NeedsSetup || st.Authenticated {
		t.Fatalf("fresh DB: want needs_setup=true authenticated=false, got %+v", st)
	}

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})
	rec = httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d, want 201", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should set a session cookie")
	}

	rec = httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("second setup = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login = %d, cookies=%d", rec.Code, len(rec.Result().Cookies()))
	}

	bad, _ := json.Marshal(map[string]string{"username": "admin", "password": "nope"})
	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(bad)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", rec.Code)
	}
}

func TestTwoFASetupEnableThenLoginEnforced(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := NewAuth(q, false)

	creds, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})

	// Setup the admin account; this returns a session cookie we reuse.
	rec := httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(creds)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d, want 201", rec.Code)
	}
	sessionCookie := sessionCookieFrom(t, rec.Result().Cookies())

	// 2FA setup returns the TOTP secret.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/setup", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	h.TwoFASetup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa setup = %d, want 200", rec.Code)
	}
	var setupResp struct {
		Secret string `json:"secret"`
	}
	json.NewDecoder(rec.Body).Decode(&setupResp)
	if setupResp.Secret == "" {
		t.Fatal("2fa setup did not return a secret")
	}

	// Enable 2FA with a valid code.
	code, err := totp.GenerateCode(setupResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	enableBody, _ := json.Marshal(map[string]string{"code": code})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/2fa/enable", bytes.NewReader(enableBody))
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	h.TwoFAEnable(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa enable = %d, want 200", rec.Code)
	}

	// Login without a code must be rejected with needs_2fa.
	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(creds)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login without code = %d, want 401", rec.Code)
	}
	var needs struct {
		Needs2FA bool `json:"needs_2fa"`
	}
	json.NewDecoder(rec.Body).Decode(&needs)
	if !needs.Needs2FA {
		t.Fatal("login without code should report needs_2fa=true")
	}

	// Login with a valid code succeeds and sets a session cookie.
	loginCode, err := totp.GenerateCode(setupResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate login code: %v", err)
	}
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass", "code": loginCode})
	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody)))
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login with code = %d, cookies=%d", rec.Code, len(rec.Result().Cookies()))
	}
}

func sessionCookieFrom(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie found")
	return nil
}

func TestSessionCookieSecureFlag(t *testing.T) {
	for _, secure := range []bool{false, true} {
		db := testutil.NewTestDB(t)
		q := generated.New(db)
		h := NewAuth(q, secure)

		body, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})
		rec := httptest.NewRecorder()
		h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("setup = %d, want 201", rec.Code)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("setup should set a session cookie")
		}
		if cookies[0].Secure != secure {
			t.Fatalf("cookie Secure = %v, want %v", cookies[0].Secure, secure)
		}
	}
}

func TestLoginRateLimited(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := NewAuth(q, false)

	setup, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})
	rec := httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(setup)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d", rec.Code)
	}

	bad, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	attempt := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(bad))
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.Login(rec, req)
		return rec.Code
	}

	for i := 0; i < 10; i++ {
		if code := attempt("203.0.113.7:1000"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, code)
		}
	}
	if code := attempt("203.0.113.7:1000"); code != http.StatusTooManyRequests {
		t.Fatalf("attempt 11 = %d, want 429", code)
	}
	// A different client IP has its own bucket.
	if code := attempt("198.51.100.9:2000"); code != http.StatusUnauthorized {
		t.Fatalf("other IP = %d, want 401", code)
	}
}
