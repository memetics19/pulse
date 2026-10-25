package alerter

import (
	"context"
	"encoding/json"
	"log"

	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/store"
)

// Alert describes an incident event to notify about.
type Alert struct {
	IncidentID int64
	Title      string
	Status     string // "detected", "investigating", "identified", "implementing", "monitoring", "resolved"
	Severity   string
	Message    string
}

// Sender dispatches a single alert over one notification channel.
type Sender interface {
	Send(ctx context.Context, cfg map[string]string, a Alert) error
}

// Dispatcher loads notifications from DB and fans out to the right Sender.
type Dispatcher struct {
	q     *store.Queries
	email Sender
	slack Sender
}

// NewDispatcher wires the senders from cfg: email through SMTP or Resend (see NewEmail),
// and Slack. cfg.AllowPrivateMonitors (PULSE_ALLOW_PRIVATE_MONITORS) decides whether a
// webhook or the SMTP relay may be on a private or internal address.
func NewDispatcher(q *store.Queries, cfg config.Config, opts ...Option) *Dispatcher {
	return &Dispatcher{
		q:     q,
		email: NewEmail(cfg),
		slack: NewSlackSender(cfg.SlackWebhookURL, cfg.AllowPrivateMonitors, opts...),
	}
}

// Notify fans out an alert to all matching notification channels.
func (d *Dispatcher) Notify(ctx context.Context, a Alert, monitorID *int64) {
	notifications, err := d.q.ListNotifications(ctx)
	if err != nil {
		log.Printf("alerter: list notifications: %v", err)
		return
	}
	for _, n := range notifications {
		// Skip if notification is for a specific monitor that doesn't match
		if n.MonitorID != nil && monitorID != nil && *n.MonitorID != *monitorID {
			continue
		}
		cfg := map[string]string{}
		if err := json.Unmarshal([]byte(n.ConfigJson), &cfg); err != nil {
			log.Printf("alerter: notification %d has invalid config: %v", n.ID, err)
			continue
		}
		var sender Sender
		switch n.Channel {
		case "email":
			sender = d.email
		case "slack":
			sender = d.slack
		default:
			continue
		}
		if err := sender.Send(ctx, cfg, a); err != nil {
			log.Printf("alerter: send via %s: %v", n.Channel, err)
		}
	}
}
