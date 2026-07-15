package alerter_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestNotifyFiltersByMonitorAndHandlesDBError(t *testing.T) {
	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	other := int64(5)
	// notification scoped to monitor 5
	q.CreateNotification(ctx, generated.CreateNotificationParams{
		Channel: "slack", ConfigJson: `{"webhook_url":"http://127.0.0.1:1"}`, MonitorID: &other,
	})
	d := alerter.NewDispatcher(q, "", "")
	// Alert for a different monitor -> the scoped notification is skipped (no panic).
	six := int64(6)
	d.Notify(ctx, alerter.Alert{Title: "t", Status: "detected"}, &six)

	// Closed DB -> ListNotifications errors and Notify returns early.
	db := testutil.NewTestDB(t)
	db.Close()
	alerter.NewDispatcher(store.New(db), "", "").Notify(ctx, alerter.Alert{}, nil)
}
