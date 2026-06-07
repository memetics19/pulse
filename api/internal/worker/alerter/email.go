package alerter

import (
	"context"
	"fmt"
	"strings"

	"github.com/resend/resend-go/v2"
)

type emailSender struct {
	client *resend.Client
}

type noopSender struct{}

func (n *noopSender) Send(_ context.Context, _ map[string]string, _ Alert) error { return nil }

// NewEmailSender returns a Resend-backed email sender, or a noop if apiKey is empty.
func NewEmailSender(apiKey string) Sender {
	if apiKey == "" {
		return &noopSender{}
	}
	return &emailSender{client: resend.NewClient(apiKey)}
}

func (s *emailSender) Send(ctx context.Context, cfg map[string]string, a Alert) error {
	to := cfg["to"]
	if to == "" {
		return nil
	}
	subject := fmt.Sprintf("[Pulse] %s — %s", strings.ToUpper(a.Status), a.Title)
	body := fmt.Sprintf("<h2>%s</h2><p><strong>Status:</strong> %s</p><p><strong>Severity:</strong> %s</p><p>%s</p>",
		a.Title, a.Status, a.Severity, a.Message)
	params := &resend.SendEmailRequest{
		From:    "pulse@notifications.pulse.app",
		To:      []string{to},
		Subject: subject,
		Html:    body,
	}
	_, err := s.client.Emails.Send(params)
	return err
}
