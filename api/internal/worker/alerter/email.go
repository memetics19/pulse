package alerter

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
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
	msg, err := renderEmail(a)
	if err != nil {
		return err
	}
	params := &resend.SendEmailRequest{
		From:    "pulse@notifications.pulse.app",
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
