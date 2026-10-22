package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A value that cannot be encoded must not panic or write a partial body. The
// status line is already out, so the failure is only logged.
func TestEncodeJSONLogsEncodeFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]any{"bad": make(chan int)})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}
