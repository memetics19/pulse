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

func TestMaintenanceCreateListStartNow(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	h := NewMaintenance(q)

	body, _ := json.Marshal(map[string]any{
		"title": "DB upgrade", "description": "brief downtime",
		"affected_monitor_ids": []int64{1, 2},
		"starts_at":            "2026-06-10T01:00:00Z", "ends_at": "2026-06-10T03:00:00Z",
		"start_now": true,
	})
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/maintenance", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create=%d", rec.Code)
	}
	var created struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	json.NewDecoder(rec.Body).Decode(&created)
	if created.Status != "in_progress" {
		t.Fatalf("start_now should be in_progress, got %q", created.Status)
	}

	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/maintenance", nil))
	if rec.Code != 200 {
		t.Fatalf("list=%d", rec.Code)
	}
}
