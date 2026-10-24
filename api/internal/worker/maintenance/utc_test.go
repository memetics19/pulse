package maintenance

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

// TestDueToStartFindsOffsetWindow reproduces issue #48. A window written with
// a +05:30 offset starts at 04:30Z, so a UTC now of 05:00Z must find it.
func TestDueToStartFindsOffsetWindow(t *testing.T) {
	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	ist := time.FixedZone("IST", 5*3600+30*60)

	if _, err := q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "ist", Status: "scheduled", AffectedMonitorIds: "[]",
		StartsAt: time.Date(2026, 10, 23, 10, 0, 0, 0, ist),
		EndsAt:   time.Date(2026, 10, 23, 12, 0, 0, 0, ist),
	}); err != nil {
		t.Fatal(err)
	}

	due, err := q.DueToStart(ctx, time.Date(2026, 10, 23, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("DueToStart: want 1 row, got %d", len(due))
	}
}

// TestSweepWithNonUTCClock covers a host whose local zone is not UTC. The
// windows are stored in UTC, as the admin UI sends them, and the sweeper is
// handed a now in a zone behind UTC.
func TestSweepWithNonUTCClock(t *testing.T) {
	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	newYork := time.FixedZone("EST", -5*3600)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

	if _, err := q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "start", Status: "scheduled", AffectedMonitorIds: "[]",
		StartsAt: now.Add(-30 * time.Minute), EndsAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "end", Status: "in_progress", AffectedMonitorIds: "[]",
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "later", Status: "scheduled", AffectedMonitorIds: "[]",
		StartsAt: now.Add(30 * time.Minute), EndsAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if err := New(q).Run(ctx, now.In(newYork)); err != nil {
		t.Fatal(err)
	}

	all, err := q.ListMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range all {
		got[m.Title] = m.Status
	}
	want := map[string]string{"start": "in_progress", "end": "completed", "later": "scheduled"}
	for title, status := range want {
		if got[title] != status {
			t.Errorf("%s: want %q, got %q", title, status, got[title])
		}
	}
}
