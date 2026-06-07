package handlers

import (
	"context"
	"encoding/json"
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
// server-rendered status page.
func Snapshot(ctx context.Context, q *generated.Queries) StatusResponse {
	groups, _ := q.ListGroups(ctx)
	monitors, _ := q.ListMonitors(ctx)
	incidents, _ := q.ListIncidents(ctx)
	theme, _ := q.GetTheme(ctx)
	if groups == nil {
		groups = []generated.MonitorGroup{}
	}
	if monitors == nil {
		monitors = []generated.Monitor{}
	}
	if incidents == nil {
		incidents = []generated.Incident{}
	}
	statuses := map[int64]string{}
	if rows, err := q.LatestStatuses(ctx); err == nil {
		for _, row := range rows {
			statuses[row.MonitorID] = row.Status
		}
	}
	return StatusResponse{
		Groups: groups, Monitors: monitors,
		Incidents: incidents, Theme: theme,
		Statuses: statuses,
	}
}

func (h *Status) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Snapshot(r.Context(), h.q))
}
