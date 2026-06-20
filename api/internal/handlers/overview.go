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

	monitors, _ := h.q.ListMonitors(ctx)
	rows, _ := h.q.LatestStatuses(ctx)

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
	agents, _ := h.q.ListAgents(ctx)
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

	incidents, _ := h.q.ListIncidents(ctx)
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
