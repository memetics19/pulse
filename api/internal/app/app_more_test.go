package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/testutil"
)

func TestWorkerLiveness(t *testing.T) {
	a := app.New()
	if a.WorkerHealthy(time.Minute) {
		t.Fatal("no beat yet -> should be unhealthy")
	}
	a.MarkWorkerAlive()
	if !a.WorkerHealthy(time.Minute) {
		t.Fatal("should be healthy right after a beat")
	}
	if a.WorkerHealthy(0) {
		t.Fatal("zero maxAge -> nothing is recent enough")
	}
}

func TestLiveDBTXForwards(t *testing.T) {
	a := app.New()
	a.SetDB(testutil.NewTestDB(t))
	tx := app.LiveDBTX(a)
	ctx := context.Background()

	if _, err := tx.ExecContext(ctx, "CREATE TABLE t (x INTEGER)"); err != nil {
		t.Fatalf("ExecContext: %v", err)
	}
	if _, err := tx.QueryContext(ctx, "SELECT x FROM t"); err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if _, err := tx.PrepareContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
}
