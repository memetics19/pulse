package web

import (
	"encoding/xml"
	"net/http"
	"strconv"
	"time"

	"github.com/memetics19/pulse/api/internal/handlers"
)

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string `xml:"title"`
	ID      string `xml:"id"`
	Updated string `xml:"updated"`
	Summary string `xml:"summary"`
}

// Feed emits an Atom 1.0 feed of incident updates for the host-resolved page,
// scoped to the incidents that affect that page's monitors.
func (p *Public) Feed(w http.ResponseWriter, r *http.Request) {
	rp, serve := ResolvePage(r.Context(), p.q, r.Host)
	if !serve {
		http.NotFound(w, r)
		return
	}
	snap := handlers.Snapshot(r.Context(), p.q)

	// page's monitor set (nil GroupIDs => all groups)
	allGroups := rp.GroupIDs == nil
	allow := map[int64]bool{}
	for _, g := range rp.GroupIDs {
		allow[g] = true
	}
	pageMonitors := map[int64]bool{}
	for _, m := range snap.Monitors {
		if m.IsActive == 0 || m.GroupID == nil {
			continue
		}
		if allGroups || allow[*m.GroupID] {
			pageMonitors[m.ID] = true
		}
	}

	title := "Status"
	if rp.Page.Title != "" {
		title = rp.Page.Title
	}

	feed := atomFeed{
		Title:   title + " — status updates",
		ID:      "urn:pulse:" + r.Host,
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
	for _, inc := range snap.Incidents {
		if !affectsPage(inc.AffectedMonitorIds, pageMonitors, allGroups) {
			continue
		}
		updated := inc.StartedAt
		summary := inc.Status + " · " + inc.Severity
		if ups, err := p.q.ListIncidentUpdates(r.Context(), inc.ID); err == nil && len(ups) > 0 {
			last := ups[len(ups)-1]
			updated = last.CreatedAt
			summary = last.Status + " — " + last.Message
		}
		feed.Entries = append(feed.Entries, atomEntry{
			Title:   inc.Title,
			ID:      idFor(r.Host, inc.ID),
			Updated: updated.UTC().Format(time.RFC3339),
			Summary: summary,
		})
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(feed)
}

func idFor(host string, incidentID int64) string {
	return "urn:pulse:" + host + ":incident:" + strconv.FormatInt(incidentID, 10)
}
