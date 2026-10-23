package handlers_test

import (
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
)

// Issue #50: a notification cannot point the server at an internal address,
// and its channel and config are checked before they are stored.
func TestNotificationsRejectUnsafeConfig(t *testing.T) {
	q := newQ(t)
	h := handlers.NewNotifications(q, false)

	bad := []struct {
		name string
		body map[string]any
	}{
		{"unknown channel", map[string]any{"channel": "sms", "config_json": `{}`}},
		{"config not an object", map[string]any{"channel": "slack", "config_json": `["x"]`}},
		{"unknown key", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://hooks.slack.com/x","proxy":"y"}`}},
		{"http webhook", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"http://hooks.slack.com/x"}`}},
		{"metadata address", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://169.254.169.254/latest/meta-data"}`}},
		{"loopback docker api", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://127.0.0.1:2375/containers/json"}`}},
		{"private lan", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://10.0.0.5/hook"}`}},
		{"credentials in url", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://user:pass@hooks.slack.com/x"}`}},
		{"email without to", map[string]any{"channel": "email", "config_json": `{}`}},
		{"email with a list", map[string]any{"channel": "email", "config_json": `{"to":"a@example.com, b@example.com"}`}},
		{"email with a name", map[string]any{"channel": "email", "config_json": `{"to":"Ops <ops@example.com>"}`}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, http.StatusBadRequest, do(h.Create, "POST", "/api/notifications", c.body).Code)
		})
	}

	good := []map[string]any{
		{"channel": "slack", "config_json": `{"webhook_url":"https://hooks.slack.com/services/T0/B0/x"}`},
		{"channel": "slack", "config_json": `{}`}, // falls back to SLACK_WEBHOOK_URL
		{"channel": "email", "config_json": `{"to":"ops@example.com"}`},
	}
	for _, body := range good {
		assert.Equal(t, http.StatusCreated, do(h.Create, "POST", "/api/notifications", body).Code, body)
	}
}

// With PULSE_ALLOW_PRIVATE_MONITORS a LAN webhook is allowed, as for monitors.
func TestNotificationsAllowPrivateWebhookWhenEnabled(t *testing.T) {
	q := newQ(t)
	h := handlers.NewNotifications(q, true)
	body := map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://10.0.0.5/hook"}`}
	assert.Equal(t, http.StatusCreated, do(h.Create, "POST", "/api/notifications", body).Code)
	// https is still required.
	body = map[string]any{"channel": "slack", "config_json": `{"webhook_url":"http://10.0.0.5/hook"}`}
	assert.Equal(t, http.StatusBadRequest, do(h.Create, "POST", "/api/notifications", body).Code)
}

// Update is checked the same way as Create.
func TestNotificationsUpdateRejectsUnsafeConfig(t *testing.T) {
	q := newQ(t)
	h := handlers.NewNotifications(q, false)
	rr := do(h.Create, "POST", "/api/notifications",
		map[string]any{"channel": "email", "config_json": `{"to":"ops@example.com"}`})
	assert.Equal(t, http.StatusCreated, rr.Code)
	upd := do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", "1"))
	}, "PUT", "/x", map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://127.0.0.1/x"}`})
	assert.Equal(t, http.StatusBadRequest, upd.Code)
}
