package web

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
)

// publicStatusResponse is the page-scoped, sanitized shape served at
// GET /api/status. It deliberately omits every internal monitor field
// (target URL, thresholds, source, external ID): the endpoint is
// unauthenticated and resolved by Host, so it must expose only what the
// corresponding public status page shows — never raw monitor configuration.
type publicStatusResponse struct {
	Overall   string           `json:"overall"`
	Groups    []publicGroup    `json:"groups"`
	Incidents []publicIncident `json:"incidents"`
	Theme     publicTheme      `json:"theme"`
}

type publicGroup struct {
	Name     string          `json:"name"`
	Monitors []publicMonitor `json:"monitors"`
}

type publicMonitor struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type publicIncident struct {
	Title    string `json:"title"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
}

// publicTheme carries only the preset name. Full branding (logo, footer) is
// rendered on the HTML status page; custom_css is intentionally excluded here.
type publicTheme struct {
	Preset string `json:"preset"`
}

// StatusJSON serves the page-scoped public status snapshot. It resolves the
// status page from the request Host (same as the HTML page) and returns only
// the monitors, incidents, and branding that page is allowed to show.
func (p *Public) StatusJSON(w http.ResponseWriter, r *http.Request) {
	rp, serve := ResolvePage(r.Context(), p.q, r.Host)
	if !serve {
		http.NotFound(w, r)
		return
	}

	snap, err := handlers.Snapshot(r.Context(), p.q)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	allGroups := rp.GroupIDs == nil
	allow := make(map[int64]bool, len(rp.GroupIDs))
	for _, id := range rp.GroupIDs {
		allow[id] = true
	}
	allowGroup := func(id int64) bool { return allGroups || allow[id] }

	groups := append([]generated.MonitorGroup(nil), snap.Groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].DisplayOrder < groups[j].DisplayOrder })

	byGroup := map[int64][]publicMonitor{}
	pageMonitors := map[int64]bool{}
	anyDown, anyDegraded := false, false
	for _, m := range snap.Monitors {
		if !m.IsActive || m.GroupID == nil || !allowGroup(*m.GroupID) {
			continue
		}
		pageMonitors[m.ID] = true
		st := snap.Statuses[m.ID]
		if st == "" {
			st = "unknown" // no check yet — not "up" (see infra-monitor fix)
		}
		switch st {
		case "down":
			anyDown = true
		case "degraded":
			anyDegraded = true
		}
		byGroup[*m.GroupID] = append(byGroup[*m.GroupID], publicMonitor{Name: m.Name, Status: st})
	}

	resp := publicStatusResponse{Theme: publicTheme{Preset: snap.Theme.Preset}}
	for _, g := range groups {
		if !allowGroup(g.ID) {
			continue
		}
		resp.Groups = append(resp.Groups, publicGroup{Name: g.Name, Monitors: byGroup[g.ID]})
	}
	switch {
	case anyDown:
		resp.Overall = "outage"
	case anyDegraded:
		resp.Overall = "degraded"
	default:
		resp.Overall = "operational"
	}
	for _, inc := range snap.Incidents {
		if inc.Status == "resolved" || !affectsPage(inc.AffectedMonitorIds, pageMonitors, allGroups) {
			continue
		}
		resp.Incidents = append(resp.Incidents, publicIncident{
			Title: inc.Title, Status: inc.Status, Severity: inc.Severity,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(resp)
}
