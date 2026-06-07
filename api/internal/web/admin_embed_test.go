package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticAssetsServesAdminIndex(t *testing.T) {
	h := StaticAssets()
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/admin/ = %d, want 200", rec.Code)
	}
}
