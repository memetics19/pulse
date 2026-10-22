package pruner_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/pruner"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestPrunerRunDBError(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pruner.New(store.New(db)).Run(context.Background()); err == nil {
		t.Fatal("Run on closed DB should error")
	}
}
