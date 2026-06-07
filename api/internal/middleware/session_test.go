package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestRequireSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RequireSession(q)(ok)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/monitors", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie = %d, want 401", rec.Code)
	}

	hash, _ := auth.HashPassword("pw-at-least-8")
	u, _ := q.CreateUser(t.Context(), generated.CreateUserParams{Username: "a", PasswordHash: hash})
	tok, _ := auth.NewSessionToken()
	_ = q.CreateSession(t.Context(), generated.CreateSessionParams{Token: tok, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})

	req := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: tok})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid session = %d, want 200", rec.Code)
	}
}
