package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	err     error
	channel string
	config  string
	alert   alerter.Alert
	calls   int
}

func (f *fakeSender) SendTo(_ context.Context, channel, configJSON string, a alerter.Alert) error {
	f.calls++
	f.channel, f.config, f.alert = channel, configJSON, a
	return f.err
}

func testNotification(h *handlers.Notifications, id string) *httptest.ResponseRecorder {
	return do(func(w http.ResponseWriter, r *http.Request) {
		h.Test(w, withChiID(r, "id", id))
	}, "POST", "/x", nil)
}

func TestNotificationTestSendsTheTestAlert(t *testing.T) {
	q := newQ(t)
	n, err := q.CreateNotification(t.Context(), generated.CreateNotificationParams{
		Channel: "email", ConfigJson: `{"to":"ops@example.com"}`,
	})
	require.NoError(t, err)
	s := &fakeSender{}
	h := handlers.NewNotifications(q, false).WithSender(s)

	rr := testNotification(h, itoa(n.ID))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"ok":true}`, rr.Body.String())
	assert.Equal(t, "email", s.channel)
	assert.Equal(t, `{"to":"ops@example.com"}`, s.config)
	assert.Equal(t, alerter.TestAlert, s.alert)

	// A second test within 10 seconds is refused without sending.
	rr = testNotification(h, itoa(n.ID))
	assert.Equal(t, http.StatusTooManyRequests, rr.Code)
	assert.NotEmpty(t, rr.Header().Get("Retry-After"))
	assert.Equal(t, 1, s.calls)
}

func TestNotificationTestReportsTheChannelError(t *testing.T) {
	q := newQ(t)
	n, err := q.CreateNotification(t.Context(), generated.CreateNotificationParams{
		Channel: "slack", ConfigJson: `{}`,
	})
	require.NoError(t, err)
	h := handlers.NewNotifications(q, false).WithSender(&fakeSender{err: errors.New("slack webhook returned 404")})

	rr := testNotification(h, itoa(n.ID))
	assert.Equal(t, http.StatusBadGateway, rr.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	assert.Equal(t, map[string]any{"ok": false, "error": "slack webhook returned 404"}, got)
}

func TestNotificationTestErrors(t *testing.T) {
	q := newQ(t)
	s := &fakeSender{}
	h := handlers.NewNotifications(q, false).WithSender(s)
	assert.Equal(t, http.StatusNotFound, testNotification(h, "999").Code)
	assert.Equal(t, http.StatusBadRequest, testNotification(h, "abc").Code)
	assert.Equal(t, 0, s.calls)

	// Without a sender the server cannot test anything.
	assert.Equal(t, http.StatusServiceUnavailable, testNotification(handlers.NewNotifications(q, false), "1").Code)
}
