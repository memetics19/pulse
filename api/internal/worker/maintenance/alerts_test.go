package maintenance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

type recordingNotifier struct {
	mu   sync.Mutex
	sent []alerter.Alert
	ids  [][]int64
}

func (r *recordingNotifier) Notify(_ context.Context, a alerter.Alert, monitorIDs ...int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, a)
	r.ids = append(r.ids, monitorIDs)
}

func TestSweepAlertsOnStartAndEnd(t *testing.T) {
	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	if _, err := q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "db upgrade", Status: "scheduled", AffectedMonitorIds: "[2]",
		StartsAt: past, EndsAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateMaintenance(ctx, store.CreateMaintenanceParams{
		Title: "cdn swap", Status: "in_progress", AffectedMonitorIds: "[]",
		StartsAt: past, EndsAt: past,
	}); err != nil {
		t.Fatal(err)
	}
	n := &recordingNotifier{}
	if err := New(q).WithNotifier(n).Run(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	if len(n.sent) != 2 {
		t.Fatalf("got %d alerts, want 2: %+v", len(n.sent), n.sent)
	}
	if n.sent[0].Title != "Maintenance: db upgrade" || n.sent[0].Status != "in_progress" {
		t.Errorf("start alert = %+v", n.sent[0])
	}
	if len(n.ids[0]) != 1 || n.ids[0][0] != 2 {
		t.Errorf("start alert monitors = %v, want [2]", n.ids[0])
	}
	if n.sent[1].Title != "Maintenance: cdn swap" || n.sent[1].Status != "completed" {
		t.Errorf("end alert = %+v", n.sent[1])
	}

	// A second sweep finds nothing due and sends nothing more.
	if err := New(q).WithNotifier(n).Run(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 {
		t.Fatalf("second sweep sent %d more alerts", len(n.sent)-2)
	}
}
