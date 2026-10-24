package alerter

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/mail"
	"strings"

	"github.com/resend/resend-go/v2"
)

type emailSender struct {
	client *resend.Client
	from   string
}

// misconfiguredSender stands in for email when RESEND_API_KEY is set but the sender is
// missing or not an address: every alert fails with the reason, so it shows in the log
// instead of disappearing.
type misconfiguredSender struct{ err error }

func (m *misconfiguredSender) Send(_ context.Context, cfg map[string]string, _ Alert) error {
	if cfg["to"] == "" {
		return nil
	}
	return m.err
}

type noopSender struct{}

func (n *noopSender) Send(_ context.Context, _ map[string]string, _ Alert) error { return nil }

// NewEmailSender returns a Resend-backed email sender, or a noop if apiKey is empty.
// from (PULSE_EMAIL_FROM) is the sender, on a domain verified in that Resend account:
// Resend refuses any other. Without a valid one every send fails with CheckEmailFrom's
// reason.
func NewEmailSender(apiKey, from string) Sender {
	if apiKey == "" {
		return &noopSender{}
	}
	if err := CheckEmailFrom(from); err != nil {
		return &misconfiguredSender{err: err}
	}
	return &emailSender{client: resend.NewClient(apiKey), from: from}
}

// CheckEmailFrom reports why from cannot be the sender of email alerts: unset, or not one
// address such as alerts@example.com or "Pulse <alerts@example.com>".
func CheckEmailFrom(from string) error {
	if strings.TrimSpace(from) == "" {
		return fmt.Errorf("email: PULSE_EMAIL_FROM is not set: set it to a sender on a domain verified in Resend")
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return fmt.Errorf("email: PULSE_EMAIL_FROM %q is not an email address: %v", from, err)
	}
	return nil
}

func (s *emailSender) Send(ctx context.Context, cfg map[string]string, a Alert) error {
	to := cfg["to"]
	if to == "" {
		return nil
	}
	msg, err := renderEmail(a)
	if err != nil {
		return err
	}
	params := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: msg.subject,
		Html:    msg.html,
		Text:    msg.text,
	}
	_, err = s.client.Emails.SendWithContext(ctx, params)
	return err
}

// The alert's fields come from people other than the admin: a push monitor's msg is
// whatever the caller of its push URL sends, check errors quote the remote server, and an
// API key with monitors:write names the monitor. html/template escapes each of them, so
// none can add markup or links to the email.
var emailHTML = template.Must(template.New("email").Parse(
	`<h2>{{.Title}}</h2><p><strong>Status:</strong> {{.Status}}</p>` +
		`<p><strong>Severity:</strong> {{.Severity}}</p><p>{{.Message}}</p>`))

type email struct{ subject, html, text string }

func renderEmail(a Alert) (email, error) {
	var b bytes.Buffer
	if err := emailHTML.Execute(&b, a); err != nil {
		return email{}, fmt.Errorf("email: render: %w", err)
	}
	return email{
		// A subject is one line: a line break in the title must not start a new header.
		subject: oneLine(fmt.Sprintf("[Pulse] %s — %s", strings.ToUpper(a.Status), a.Title)),
		html:    b.String(),
		text:    fmt.Sprintf("%s\n\nStatus: %s\nSeverity: %s\n\n%s\n", a.Title, a.Status, a.Severity, a.Message),
	}, nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
