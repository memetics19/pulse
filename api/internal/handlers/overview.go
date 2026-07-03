package handlers

import (
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
)

// Overview serves the admin dashboard summary endpoint.
type Overview struct{ q *generated.Queries }

func NewOverview(q *generated.Queries) *Overview { return &Overview{q: q} }

type attentionItem struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func (h *Overview) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	monitors, err := h.q.ListMonitors(ctx)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	rows, err := h.q.LatestStatuses(ctx)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	status := make(map[int64]string, len(rows))
	for _, row := range rows {
		status[row.MonitorID] = row.Status
	}

	var up, degraded, down int
	attention := []attentionItem{}
	for _, m := range monitors {
		if !m.IsActive {
			continue
		}
		s := status[m.ID]
		if s == "" {
			s = "up"
		}
		switch s {
		case "down":
			down++
			attention = append(attention, attentionItem{Kind: "down", Message: m.Name + " is down"})
		case "degraded":
			degraded++
		default:
			up++
		}
	}

	// InfraAgent: Name string, IsActive bool, LastSeenAt *time.Time
	agents, err := h.q.ListAgents(ctx)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	for _, a := range agents {
		if !a.IsActive || a.LastSeenAt == nil {
			continue
		}
		if time.Since(*a.LastSeenAt) > 2*time.Minute {
			attention = append(attention, attentionItem{
				Kind:    "agent_offline",
				Message: "Agent " + a.Name + " is offline",
			})
		}
	}

	incidents, err := h.q.ListIncidents(ctx)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	active := []generated.Incident{}
	for _, i := range incidents {
		if i.Status != "resolved" {
			active = append(active, i)
		}
	}

	overall := "operational"
	if down > 0 {
		overall = "outage"
	} else if degraded > 0 {
		overall = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"overall":          overall,
		"counts":           map[string]int{"up": up, "degraded": degraded, "down": down},
		"total_monitors":   len(monitors),
		"agent_count":      len(agents),
		"active_incidents": active,
		"attention":        attention,
	})
}
