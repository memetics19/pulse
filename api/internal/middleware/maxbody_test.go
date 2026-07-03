package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaxBodyRejectsOversizedBody(t *testing.T) {
	h := MaxBody(64)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	small := bytes.NewReader([]byte(`{"ok":true}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", small))
	if rec.Code != http.StatusOK {
		t.Fatalf("small body = %d, want 200", rec.Code)
	}

	big := bytes.NewReader(append([]byte(`{"pad":"`), append(bytes.Repeat([]byte("x"), 128), '"', '}')...))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", big))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized body = %d, want 400", rec.Code)
	}
}
