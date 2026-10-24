package alerter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCheckEmailFrom(t *testing.T) {
	for _, ok := range []string{"alerts@example.com", "Pulse <alerts@example.com>"} {
		if err := CheckEmailFrom(ok); err != nil {
			t.Errorf("CheckEmailFrom(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "  ", "alerts", "a@example.com, b@example.com"} {
		if err := CheckEmailFrom(bad); err == nil || !strings.Contains(err.Error(), "PULSE_EMAIL_FROM") {
			t.Errorf("CheckEmailFrom(%q) = %v, want an error naming PULSE_EMAIL_FROM", bad, err)
		}
	}
}

func TestEmailWithoutSenderFailsLoudly(t *testing.T) {
	s := NewEmailSender("re_fake_key", "")
	err := s.Send(context.Background(), map[string]string{"to": "ops@example.com"}, Alert{Title: "API"})
	if err == nil || !strings.Contains(err.Error(), "PULSE_EMAIL_FROM is not set") {
		t.Fatalf("send without a sender = %v, want the reason", err)
	}
	// No recipient: nothing to send, so no error either.
	if err := s.Send(context.Background(), map[string]string{}, Alert{}); err != nil {
		t.Fatalf("no recipient = %v, want nil", err)
	}
}

// The request Resend gets carries the configured sender, the escaped HTML and the text part.
func TestEmailUsesTheConfiguredSender(t *testing.T) {
	var req map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()

	s := NewEmailSender("re_fake_key", "Pulse <alerts@example.com>").(*emailSender)
	s.client.BaseURL, _ = url.Parse(srv.URL + "/")
	if err := s.Send(context.Background(), map[string]string{"to": "ops@example.com"}, Alert{
		Title: "API", Status: "detected", Severity: "major", Message: "<b>down</b>",
	}); err != nil {
		t.Fatal(err)
	}
	if req["from"] != "Pulse <alerts@example.com>" {
		t.Errorf("from = %v, want the configured sender", req["from"])
	}
	if html, _ := req["html"].(string); !strings.Contains(html, "&lt;b&gt;down&lt;/b&gt;") {
		t.Errorf("html = %q, want the message escaped", html)
	}
	if text, _ := req["text"].(string); !strings.Contains(text, "<b>down</b>") {
		t.Errorf("text = %q, want the plain text part", text)
	}
}
