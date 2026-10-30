package alerter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		if err := d.SendTo(ctx, n.Channel, n.ConfigJson, a); err != nil {
			if errors.Is(err, errUnknownChannel) {
				continue
			}
			log.Printf("alerter: notification %d via %s: %v", n.ID, n.Channel, err)
		}
	}
}

var errUnknownChannel = errors.New("unknown channel")

// TestAlert is the fixed alert a test notification sends. It says plainly that it
// is a test, so nobody mistakes it for an outage.
var TestAlert = Alert{
	Title:    "Test notification",
	Status:   "test",
	Severity: "info",
	Message:  "This is a test notification from Pulse. If you can read it, this channel works. No incident is open.",
}

// SendTo sends a through one notification's channel, with its stored config, and
// returns the sender's error. An unknown channel or a config that is not a JSON
// object of strings is an error too.
func (d *Dispatcher) SendTo(ctx context.Context, channel, configJSON string, a Alert) error {
	var sender Sender
	switch channel {
	case "email":
		sender = d.email
	case "slack":
		sender = d.slack
	default:
		return fmt.Errorf("%w %q", errUnknownChannel, channel)
	}
	cfg := map[string]string{}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}
	return sender.Send(ctx, cfg, a)
}
