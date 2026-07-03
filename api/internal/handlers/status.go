package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memetics19/pulse/api/internal/generated"
)

type Status struct{ q *generated.Queries }

func NewStatus(q *generated.Queries) *Status { return &Status{q: q} }

type StatusResponse struct {
	Groups    []generated.MonitorGroup `json:"groups"`
	Monitors  []generated.Monitor      `json:"monitors"`
	Incidents []generated.Incident     `json:"incidents"`
	Theme     generated.ThemeConfig    `json:"theme"`
	Statuses  map[int64]string         `json:"statuses"`
}

// Snapshot loads the public status data shared by the JSON API and the
// server-rendered status page. It returns the first database error so
// callers can respond with a real error instead of an empty page.
func Snapshot(ctx context.Context, q *generated.Queries) (StatusResponse, error) {
	groups, err := q.ListGroups(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	monitors, err := q.ListMonitors(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	incidents, err := q.ListIncidents(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	// No theme row yet is normal on a fresh instance; fall back to defaults.
	theme, err := q.GetTheme(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return StatusResponse{}, err
	}
	if groups == nil {
		groups = []generated.MonitorGroup{}
	}
	if monitors == nil {
		monitors = []generated.Monitor{}
	}
	if incidents == nil {
		incidents = []generated.Incident{}
	}
	rows, err := q.LatestStatuses(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	statuses := map[int64]string{}
	for _, row := range rows {
		statuses[row.MonitorID] = row.Status
	}
	return StatusResponse{
		Groups: groups, Monitors: monitors,
		Incidents: incidents, Theme: theme,
		Statuses: statuses,
	}, nil
}

func (h *Status) Get(w http.ResponseWriter, r *http.Request) {
	snap, err := Snapshot(r.Context(), h.q)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}
