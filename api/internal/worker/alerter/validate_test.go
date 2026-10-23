package alerter_test

import (
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/stretchr/testify/assert"
)

func TestValidateNotification(t *testing.T) {
	cases := []struct {
		name         string
		channel, cfg string
		allowPrivate bool
		ok           bool
	}{
		{"slack public https", "slack", `{"webhook_url":"https://hooks.slack.com/services/T/B/x"}`, false, true},
		{"slack empty uses env", "slack", `{}`, false, true},
		{"slack blank config", "slack", ``, false, true},
		{"email ok", "email", `{"to":"ops@example.com"}`, false, true},
		{"lan allowed when private allowed", "slack", `{"webhook_url":"https://192.168.1.10/hook"}`, true, true},
		{"unknown channel", "pagerduty", `{}`, false, false},
		{"not json", "slack", `{`, false, false},
		{"not strings", "slack", `{"webhook_url":1}`, false, false},
		{"unknown key", "email", `{"to":"ops@example.com","bcc":"x@example.com"}`, false, false},
		{"http", "slack", `{"webhook_url":"http://hooks.slack.com/x"}`, false, false},
		{"ftp", "slack", `{"webhook_url":"ftp://hooks.slack.com/x"}`, false, false},
		{"no host", "slack", `{"webhook_url":"https:///x"}`, false, false},
		{"bad url", "slack", `{"webhook_url":"https://[::1"}`, false, false},
		{"credentials", "slack", `{"webhook_url":"https://u:p@hooks.slack.com/x"}`, false, false},
		{"metadata", "slack", `{"webhook_url":"https://169.254.169.254/"}`, false, false},
		{"loopback", "slack", `{"webhook_url":"https://localhost:2375/x"}`, false, false},
		{"lan refused by default", "slack", `{"webhook_url":"https://192.168.1.10/hook"}`, false, false},
		{"email missing", "email", `{}`, false, false},
		{"email not an address", "email", `{"to":"not-an-address"}`, false, false},
		{"email display name", "email", `{"to":"Ops <ops@example.com>"}`, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := alerter.ValidateNotification(c.channel, c.cfg, c.allowPrivate)
			if c.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
