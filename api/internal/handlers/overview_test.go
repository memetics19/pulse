package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestOverviewEmptyDB(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := NewOverview(generated.New(db))
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/overview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var o struct {
		Overall string `json:"overall"`
	}
	json.NewDecoder(rec.Body).Decode(&o)
	if o.Overall != "operational" {
		t.Fatalf("empty DB overall=%q want operational", o.Overall)
	}
}
