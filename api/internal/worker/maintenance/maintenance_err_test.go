package maintenance_test

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/maintenance"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestMaintenanceRunDBError(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.New(store.New(db)).Run(context.Background(), time.Now()); err == nil {
		t.Fatal("Run on closed DB should error")
	}
}
