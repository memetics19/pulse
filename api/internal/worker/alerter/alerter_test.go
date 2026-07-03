package alerter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/stretchr/testify/assert"
)

func TestSlackSender_SendsPayload(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		received = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := alerter.NewSlackSender(srv.URL)
	err := s.Send(context.Background(), map[string]string{}, alerter.Alert{
		IncidentID: 1,
		Title:      "API down",
		Status:     "detected",
		Severity:   "critical",
		Message:    "Monitor returned 503",
	})
	assert.NoError(t, err)
	assert.Contains(t, string(received), "API down")
}

func TestEmailSender_NoopWhenNoKey(t *testing.T) {
	s := alerter.NewEmailSender("")
	err := s.Send(context.Background(), map[string]string{}, alerter.Alert{Title: "test", Status: "detected"})
	assert.NoError(t, err)
}

func TestSlackSender_ErrorsWhenNoURL(t *testing.T) {
	// A missing webhook URL used to be a silent noop, which hid
	// misconfiguration; it is now a real error.
	s := alerter.NewSlackSender("")
	err := s.Send(context.Background(), map[string]string{}, alerter.Alert{Title: "test", Status: "detected"})
	assert.Error(t, err)
}

func TestSlackSender_UsesPerNotificationWebhookURL(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Sender constructed with NO env webhook: per-notification config must work.
	s := alerter.NewSlackSender("")
	err := s.Send(context.Background(), map[string]string{"webhook_url": srv.URL}, alerter.Alert{
		Title: "API down", Status: "detected",
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, hits)

	// No URL anywhere: a real error, not a silent noop.
	err = s.Send(context.Background(), map[string]string{}, alerter.Alert{Title: "x"})
	assert.Error(t, err)
}
