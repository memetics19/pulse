package alerter

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

// configKeys lists, per channel, the config_json keys a notification may carry.
// Anything else is rejected, so a notification cannot smuggle settings a sender
// might start reading later.
var configKeys = map[string][]string{
	"email": {"to"},
	"slack": {"webhook_url"},
}

// ValidateNotification checks a notification before it is stored: the channel
// is known, config_json is a JSON object of strings with only that channel's
// keys, and each value is safe to use. allowPrivate is PULSE_ALLOW_PRIVATE_MONITORS:
// a webhook may point at a private or internal address only when it is set,
// the same rule the monitor checkers follow.
func ValidateNotification(channel, configJSON string, allowPrivate bool) error {
	allowed, ok := configKeys[channel]
	if !ok {
		return fmt.Errorf("channel must be one of %s", strings.Join(channelNames(), ", "))
	}
	cfg := map[string]string{}
	if strings.TrimSpace(configJSON) != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return fmt.Errorf("config_json must be a JSON object of strings: %v", err)
		}
	}
	for k := range cfg {
		if !contains(allowed, k) {
			return fmt.Errorf("config_json: %q is not a %s setting (allowed: %s)", k, channel, strings.Join(allowed, ", "))
		}
	}
	switch channel {
	case "email":
		if err := validateEmail(cfg["to"]); err != nil {
			return err
		}
	case "slack":
		// Empty means "use SLACK_WEBHOOK_URL"; a value must be a safe webhook.
		if u := cfg["webhook_url"]; u != "" {
			if err := ValidateWebhookURL(u, allowPrivate); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateWebhookURL rejects a webhook URL that is not https, has no host, carries
// credentials, or resolves to a forbidden address. It is checked again before every
// send, and the dial-time guard in newWebhookClient still applies.
func ValidateWebhookURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook_url is not a valid URL: %v", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook_url must use https")
	}
	if u.User != nil {
		return fmt.Errorf("webhook_url must not contain credentials")
	}
	if err := netguard.ValidateURL(raw, allowPrivate); err != nil {
		return fmt.Errorf("webhook_url: %v", err)
	}
	return nil
}

func validateEmail(to string) error {
	if to == "" {
		return fmt.Errorf("config_json: an email notification needs \"to\"")
	}
	addr, err := mail.ParseAddress(to)
	if err != nil || addr.Address != to {
		return fmt.Errorf("config_json: \"to\" must be one plain email address")
	}
	return nil
}

// Option changes how a sender is built.
type Option func(*senderOptions)

type senderOptions struct {
	client **http.Client
}

// WithHTTPClient replaces the webhook client, for tests that post to an
// httptest TLS server (its client trusts the test certificate). It drops the
// dial guard, so production code never passes it. ValidateWebhookURL still
// runs before every send.
func WithHTTPClient(c *http.Client) Option {
	return func(o *senderOptions) { *o.client = c }
}

// newWebhookClient sends webhooks through the netguard dial guard (no private or
// internal addresses unless allowPrivate, checked at connect time, so DNS answers
// that change after validation are caught too) and follows no redirects: a 3xx is
// returned as is and reported as a failed send.
func newWebhookClient(allowPrivate bool) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
				Control: netguard.DialControl(allowPrivate),
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func channelNames() []string {
	names := make([]string, 0, len(configKeys))
	for k := range configKeys {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
