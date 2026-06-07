package alerter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type slackSender struct {
	webhookURL string
}

// NewSlackSender returns a Slack webhook sender, or a noop if webhookURL is empty.
func NewSlackSender(webhookURL string) Sender {
	if webhookURL == "" {
		return &noopSender{}
	}
	return &slackSender{webhookURL: webhookURL}
}

func (s *slackSender) Send(ctx context.Context, cfg map[string]string, a Alert) error {
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

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}
