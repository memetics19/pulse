package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
)

func TestSetupStateAndComplete(t *testing.T) {
	dataDir := t.TempDir()
	a := app.New()
	h := NewSetup(a, dataDir, false)

	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
	var st struct {
		Configured bool `json:"configured"`
	}
	json.NewDecoder(rec.Body).Decode(&st)
	if st.Configured {
		t.Fatal("fresh app: configured should be false")
	}

	body, _ := json.Marshal(map[string]any{
		"name": "Shreeda", "email": "s@b.c", "username": "admin", "password": "supersecret",
		"site_name": "Acme", "logo_url": "", "sqlite_path": dataDir + "/pulse.db",
	})
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup complete = %d, want 201", rec.Code)
	}
	if !a.Configured() {
		t.Fatal("app should be configured after setup")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should log the admin in (session cookie)")
	}

	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("second setup = %d, want 403", rec.Code)
	}
}
