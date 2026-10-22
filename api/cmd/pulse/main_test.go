package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
)

func TestServerServesHealthz(t *testing.T) {
	db := testutil.NewTestDB(t)
	a := app.New()
	a.SetDB(db)
	a.MarkWorkerAlive() // simulate a live worker
	h := server.New(a, t.TempDir(), config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", rec.Code)
	}
}

// A configured app whose worker has never beaten (or has gone stale) must fail
// the health check so orchestration restarts a monitoring-dead container.
func TestHealthzUnhealthyWhenWorkerDead(t *testing.T) {
	db := testutil.NewTestDB(t)
	a := app.New()
	a.SetDB(db) // configured, but MarkWorkerAlive never called
	h := server.New(a, t.TempDir(), config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz = %d, want 503", rec.Code)
	}
}
