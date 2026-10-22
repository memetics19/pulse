package alerter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
)

func TestSlackSender(t *testing.T) {
	var got atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := alerter.NewSlackSender("")
	ctx := context.Background()
	a := alerter.Alert{Title: "down", Status: "detected", Severity: "major", Message: "m"}

	if err := s.Send(ctx, map[string]string{"webhook_url": srv.URL}, a); err != nil {
		t.Fatalf("slack send ok: %v", err)
	}
	if got.Load() != 1 {
		t.Fatal("webhook not called")
	}
	// No URL anywhere -> error.
	if err := s.Send(ctx, map[string]string{}, a); err == nil {
		t.Fatal("expected error when no webhook_url")
	}

	// Non-2xx -> error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if err := s.Send(ctx, map[string]string{"webhook_url": bad.URL}, a); err == nil {
		t.Fatal("expected error on non-200 webhook")
	}
}

func TestEmailSenderNoop(t *testing.T) {
	// Empty API key -> noop sender that never errors.
	s := alerter.NewEmailSender("")
	if err := s.Send(context.Background(), map[string]string{"to": "x@y.com"}, alerter.Alert{}); err != nil {
		t.Fatalf("noop email should not error: %v", err)
	}
	// Real sender but no recipient -> returns nil before calling the API.
	s2 := alerter.NewEmailSender("re_fake_key")
	if err := s2.Send(context.Background(), map[string]string{}, alerter.Alert{}); err != nil {
		t.Fatalf("email with no recipient should be a no-op: %v", err)
	}
}

func TestDispatcherNotifyFansOut(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	q := store.New(testutil.NewTestDB(t))
	ctx := context.Background()
	if _, err := q.CreateNotification(ctx, generated.CreateNotificationParams{
		Channel: "slack", ConfigJson: `{"webhook_url":"` + srv.URL + `"}`,
	}); err != nil {
		t.Fatal(err)
	}
	// An unknown channel row must be skipped, not crash.
	q.CreateNotification(ctx, generated.CreateNotificationParams{Channel: "email", ConfigJson: `bad json`})

	d := alerter.NewDispatcher(q, "", "")
	d.Notify(ctx, alerter.Alert{Title: "t", Status: "detected", Severity: "major"}, nil)

	if hits.Load() != 1 {
		t.Fatalf("expected slack webhook hit once, got %d", hits.Load())
	}
}
