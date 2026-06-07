package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestAPIKeyCreateThenListHidesKey(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	h := NewAPIKeys(q)

	body, _ := json.Marshal(map[string]any{"name": "ci", "scopes": []string{"monitors:read"}})
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create=%d", rec.Code)
	}
	var created struct {
		Key    string   `json:"key"`
		Prefix string   `json:"prefix"`
		Scopes []string `json:"scopes"`
	}
	json.NewDecoder(rec.Body).Decode(&created)
	if !strings.HasPrefix(created.Key, "pulse_live_") {
		t.Fatalf("missing full key: %q", created.Key)
	}
	if len(created.Scopes) != 1 || created.Scopes[0] != "monitors:read" {
		t.Fatalf("scopes round-trip failed: %v", created.Scopes)
	}

	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/keys", nil))
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, created.Key) {
		t.Fatal("list must NOT contain the full key")
	}
	if !strings.Contains(bodyStr, created.Prefix) {
		t.Fatal("list should contain the prefix")
	}
}
