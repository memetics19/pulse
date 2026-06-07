package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestPagesCreateList(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	h := NewPages(q)

	body, _ := json.Marshal(map[string]any{"domain": "acme.com", "title": "Acme", "published": true, "group_ids": []int64{1, 2}})
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/pages", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/pages", nil))
	var pages []struct {
		Domain    string  `json:"domain"`
		IsDefault bool    `json:"is_default"`
		GroupIDs  []int64 `json:"group_ids"`
	}
	json.NewDecoder(rec.Body).Decode(&pages)
	if len(pages) < 2 {
		t.Fatalf("want default + created, got %d", len(pages))
	}
	var found bool
	for _, p := range pages {
		if p.Domain == "acme.com" && len(p.GroupIDs) == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("created page with its groups not listed")
	}
}
