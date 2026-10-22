package rollup_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/rollup"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestRollupRunDBError(t *testing.T) {
	db := testutil.NewTestDB(t)
	db.Close()
	if err := rollup.New(store.New(db)).Run(context.Background()); err == nil {
		t.Fatal("Run on closed DB should error")
	}
}
