package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
)

// NotificationSender sends one alert through a notification's channel and returns
// the channel's error. *alerter.Dispatcher implements it.
type NotificationSender interface {
	SendTo(ctx context.Context, channel, configJSON string, a alerter.Alert) error
}

// testEvery is how often one notification may be tested. It keeps the endpoint
// from being used to flood a webhook or an inbox.
const testEvery = 10 * time.Second

// testTimeout bounds one test send. Each sender has its own, shorter timeout too.
const testTimeout = 45 * time.Second

// WithSender sets what Test sends through. Without it Test answers 503.
func (h *Notifications) WithSender(s NotificationSender) *Notifications {
	h.sender = s
	return h
}

type testResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Test sends alerter.TestAlert through one notification's channel and reports the
// result: 200 {"ok":true}, or 502 {"ok":false,"error":"..."} with the channel's
// error. Each notification may be tested once per testEvery. Sooner is 429.
func (h *Notifications) Test(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if h.sender == nil {
		http.Error(w, "notifications cannot be sent by this server", http.StatusServiceUnavailable)
		return
	}
	n, err := h.q.GetNotification(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "notification not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if wait := h.limiter.take(id); wait > 0 {
		secs := int(math.Ceil(wait.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, testResult{
			Error: fmt.Sprintf("this notification was tested a moment ago, try again in %d seconds", secs),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	if err := h.sender.SendTo(ctx, n.Channel, n.ConfigJson, alerter.TestAlert); err != nil {
		log.Printf("notifications: test of notification %d via %s: %v", n.ID, n.Channel, err)
		writeJSON(w, http.StatusBadGateway, testResult{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true})
}

// testLimiter allows one test per notification per every.
type testLimiter struct {
	mu    sync.Mutex
	every time.Duration
	now   func() time.Time
	last  map[int64]time.Time
}

func newTestLimiter(every time.Duration) *testLimiter {
	return &testLimiter{every: every, now: time.Now, last: map[int64]time.Time{}}
}

// take records a test of id and returns 0, or returns how long to wait when id
// was tested less than every ago.
func (l *testLimiter) take(id int64) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if last, ok := l.last[id]; ok {
		if wait := l.every - now.Sub(last); wait > 0 {
			return wait
		}
	}
	// Forget old entries so the map stays small.
	if len(l.last) > 1000 {
		for k, t := range l.last {
			if now.Sub(t) >= l.every {
				delete(l.last, k)
			}
		}
	}
	l.last[id] = now
	return 0
}
