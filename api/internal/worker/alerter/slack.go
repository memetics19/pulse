package alerter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type slackSender struct {
	webhookURL   string
	allowPrivate bool
	client       *http.Client
}

// NewSlackSender returns a Slack webhook sender. webhookURL (from the
// SLACK_WEBHOOK_URL env) is only the fallback: each notification may carry
// its own webhook_url in its config, so the sender must exist even when the
// env var is unset. allowPrivate is PULSE_ALLOW_PRIVATE_MONITORS: without it a
// webhook cannot reach a private or internal address (see newWebhookClient).
func NewSlackSender(webhookURL string, allowPrivate bool, opts ...Option) Sender {
	s := &slackSender{webhookURL: webhookURL, allowPrivate: allowPrivate, client: newWebhookClient(allowPrivate)}
	for _, o := range opts {
		o(&senderOptions{client: &s.client})
	}
	return s
}

func (s *slackSender) Send(ctx context.Context, cfg map[string]string, a Alert) error {
	url := cfg["webhook_url"]
	if url == "" {
		url = s.webhookURL
	}
	if url == "" {
		return fmt.Errorf("slack: no webhook_url in notification config and SLACK_WEBHOOK_URL is unset")
	}
	// Checked again here: a notification stored before validation existed, or
	// SLACK_WEBHOOK_URL itself, must not reach an address the guard forbids.
	if err := ValidateWebhookURL(url, s.allowPrivate); err != nil {
		return fmt.Errorf("slack: %v", err)
	}
	emoji := map[string]string{
		"detected":      ":red_circle:",
		"investigating": ":large_yellow_circle:",
		"identified":    ":large_blue_circle:",
		"implementing":  ":large_purple_circle:",
		"monitoring":    ":large_yellow_circle:",
		"resolved":      ":large_green_circle:",
	}
	icon := emoji[a.Status]
	if icon == "" {
		icon = ":white_circle:"
	}
	text := fmt.Sprintf("%s *[%s]* %s\n%s", icon, a.Status, a.Title, a.Message)
	payload := map[string]string{"text": text}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	// Only the status code matters, so a failed close loses nothing.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}
