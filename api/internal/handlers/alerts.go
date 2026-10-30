package handlers

import (
	"context"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
)

// Notifier sends an alert to the notification channels without waiting for the
// sends. *alerter.Dispatcher implements it.
type Notifier interface {
	Notify(ctx context.Context, a alerter.Alert, monitorIDs ...int64)
}
