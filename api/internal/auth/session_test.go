package auth

import (
	"net/http/httptest"
	"testing"
)

func TestNewSessionTokenIsRandom(t *testing.T) {
	a, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a == b {
		t.Fatalf("tokens must be non-empty and unique: %q %q", a, b)
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, "tok123", false)
	c := rec.Result().Cookies()
	if len(c) != 1 || c[0].Name != SessionCookieName || c[0].Value != "tok123" || !c[0].HttpOnly {
		t.Fatalf("unexpected cookie: %+v", c)
	}
	rec2 := httptest.NewRecorder()
	ClearSessionCookie(rec2)
	c2 := rec2.Result().Cookies()
	if len(c2) != 1 || c2[0].MaxAge >= 0 {
		t.Fatalf("expected an expiring cookie, got: %+v", c2)
	}
}
