package handlers_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
)

func TestSnapshotIncludesStatuses(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	snap := handlers.Snapshot(context.Background(), q)
	if snap.Statuses == nil {
		t.Fatal("Statuses map should be non-nil")
	}
}
