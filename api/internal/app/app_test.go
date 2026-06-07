package app

import (
	"testing"

	"github.com/memetics19/pulse/api/testutil"
)

func TestAppConfiguredFlips(t *testing.T) {
	a := New()
	if a.Configured() {
		t.Fatal("new app must be unconfigured")
	}
	if a.Queries() != nil {
		t.Fatal("no queries before configuration")
	}
	db := testutil.NewTestDB(t)
	a.SetDB(db)
	if !a.Configured() || a.Queries() == nil || a.DB() == nil {
		t.Fatal("SetDB must make the app configured with queries + db")
	}
}
