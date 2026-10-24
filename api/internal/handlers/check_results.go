package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
)

type CheckResults struct {
	q *generated.Queries
}

func NewCheckResults(q *generated.Queries) *CheckResults {
	return &CheckResults{q: q}
}

func (h *CheckResults) List(w http.ResponseWriter, r *http.Request) {
	monitorIDStr := chi.URLParam(r, "monitorID")
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid monitorID", http.StatusBadRequest)
		return
	}

	days := daysParam(r, 1)

	since := time.Now().AddDate(0, 0, -days)
	results, err := h.q.ListCheckResults(r.Context(), generated.ListCheckResultsParams{
		MonitorID: monitorID,
		CheckedAt: since,
		Limit:     int64(1000),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []generated.CheckResult{}
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, results)
}

func (h *CheckResults) Uptime(w http.ResponseWriter, r *http.Request) {
	monitorIDStr := chi.URLParam(r, "monitorID")
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid monitorID", http.StatusBadRequest)
		return
	}

	days := daysParam(r, maxCheckDays)

	since := time.Now().AddDate(0, 0, -days)
	uptimePct, err := h.q.UptimePercent(r.Context(), generated.UptimePercentParams{
		MonitorID: monitorID,
		CheckedAt: since,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, map[string]interface{}{
		"monitor_id": monitorID,
		"days":       days,
		"uptime_pct": uptimePct,
	})
}

// maxCheckDays caps the ?days window. It matches the longest range the status
// page and admin UI offer, and keeps one request from scanning the whole
// check_results table.
const maxCheckDays = 90

// daysParam reads ?days, falling back to def when it is missing or not a
// positive integer, and clamps it to maxCheckDays.
func daysParam(r *http.Request, def int) int {
	d, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || d <= 0 {
		return def
	}
	return min(d, maxCheckDays)
}
