package incident_test

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestDetectorSuppressesDuringMaintenance(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	ctx := context.Background()

	mon, err := q.CreateMonitor(ctx, store.CreateMonitorParams{
		Name: "m", Url: "http://x", Type: "http", IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Active maintenance window covering the monitor.
	now := time.Now()
	_, err = q.CreateMaintenance(ctx, generated.CreateMaintenanceParams{
		Title: "mw", Status: "in_progress", AffectedMonitorIds: "[" + itoa(mon.ID) + "]",
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	d := incident.NewDetector(q)
	// Two consecutive downs would normally open an incident; maintenance suppresses it.
	d.MaybeCreateIncident(ctx, mon.ID, "down")
	created, err := d.MaybeCreateIncident(ctx, mon.ID, "down")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("incident should be suppressed during maintenance")
	}
}

func itoa(n int64) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
