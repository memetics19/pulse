package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestPublicPageRendersStatus(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := NewPublic(q)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/ = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "All systems operational") {
		t.Fatalf("expected operational text, got:\n%s", body)
	}
	if !strings.Contains(body, "<html") {
		t.Fatal("expected an HTML document")
	}
}
