package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckResultsDaysParam(t *testing.T) {
	q := newQ(t)
	mon, err := q.CreateMonitor(context.Background(), generated.CreateMonitorParams{
		Name: "m", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)
	h := handlers.NewCheckResults(q)
	id := itoa(mon.ID)
	// custom, zero, and non-numeric days all resolve to 200 (bad values fall
	// back to the default window).
	for _, days := range []string{"?days=7", "?days=0", "?days=abc", ""} {
		rr := do(func(w http.ResponseWriter, r *http.Request) {
			h.List(w, withChiID(r, "monitorID", id))
		}, "GET", "/x"+days, nil)
		assert.Equal(t, http.StatusOK, rr.Code, "days=%q", days)

		ur := do(func(w http.ResponseWriter, r *http.Request) {
			h.Uptime(w, withChiID(r, "monitorID", id))
		}, "GET", "/x"+days, nil)
		assert.Equal(t, http.StatusOK, ur.Code, "uptime days=%q", days)
	}
}
