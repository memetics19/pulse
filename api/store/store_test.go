package store_test

import (
	"testing"

	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestNew(t *testing.T) {
	if store.New(testutil.NewTestDB(t)) == nil {
		t.Fatal("store.New returned nil")
	}
}
