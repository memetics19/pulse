package alerter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestNotifyFiltersByMonitorAndHandlesDBError(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	mon, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "scoped", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000,
		IsActive: true, Source: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	// A notification scoped to that monitor.
	if _, err := q.CreateNotification(ctx, generated.CreateNotificationParams{
		Channel: "slack", ConfigJson: `{"webhook_url":"` + srv.URL + `"}`, MonitorID: &mon.ID,
	}); err != nil {
		t.Fatal(err)
	}
	d := alerter.NewDispatcher(q, config.Config{AllowPrivateMonitors: true}, alerter.WithHTTPClient(srv.Client()))

	// Alert for a different monitor -> the scoped notification is skipped.
	other := mon.ID + 1
	d.Notify(ctx, alerter.Alert{Title: "t", Status: "detected"}, &other)
	if got := hits.Load(); got != 0 {
		t.Fatalf("notification scoped to monitor %d fired for monitor %d (%d hits)", mon.ID, other, got)
	}

	// Alert for the scoped monitor -> it fires.
	d.Notify(ctx, alerter.Alert{Title: "t", Status: "detected"}, &mon.ID)
	if got := hits.Load(); got != 1 {
		t.Fatalf("scoped notification hits = %d, want 1", got)
	}

	// Closed DB -> ListNotifications errors and Notify returns early.
	db := testutil.NewTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	alerter.NewDispatcher(store.New(db), config.Config{}).Notify(ctx, alerter.Alert{}, nil)
}
