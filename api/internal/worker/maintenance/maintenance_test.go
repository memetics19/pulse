package maintenance

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestSweepTransitions(t *testing.T) {
	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	// scheduled, already due to start
	_, _ = q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "a", Status: "scheduled", AffectedMonitorIds: "[]",
		StartsAt: past, EndsAt: time.Now().Add(time.Hour),
	})
	// in_progress, already due to end
	_, _ = q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "b", Status: "in_progress", AffectedMonitorIds: "[]",
		StartsAt: past, EndsAt: past,
	})
	s := New(q)
	if err := s.Run(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	all, _ := q.ListMaintenance(ctx)
	got := map[string]string{}
	for _, m := range all {
		got[m.Title] = m.Status
	}
	if got["a"] != "in_progress" {
		t.Fatalf("a: want in_progress, got %q", got["a"])
	}
	if got["b"] != "completed" {
		t.Fatalf("b: want completed, got %q", got["b"])
	}
}
