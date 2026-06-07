package handlers

import (
	"encoding/json"
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

	days := 1
	if dStr := r.URL.Query().Get("days"); dStr != "" {
		if d, err := strconv.Atoi(dStr); err == nil && d > 0 {
			days = d
		}
	}

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
	json.NewEncoder(w).Encode(results)
}

func (h *CheckResults) Uptime(w http.ResponseWriter, r *http.Request) {
	monitorIDStr := chi.URLParam(r, "monitorID")
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid monitorID", http.StatusBadRequest)
		return
	}

	days := 90
	if dStr := r.URL.Query().Get("days"); dStr != "" {
		if d, err := strconv.Atoi(dStr); err == nil && d > 0 {
			days = d
		}
	}

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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"monitor_id": monitorID,
		"days":       days,
		"uptime_pct": uptimePct,
	})
}
