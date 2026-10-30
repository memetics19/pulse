package web

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
)

// Badge colours, shields.io flat style.
const (
	badgeGreen  = "#4c1"
	badgeYellow = "#dfb317"
	badgeRed    = "#e05d44"
	badgeGrey   = "#9f9f9f"
	badgeLabel  = "#555"

	defaultBadgeRange = "30d"
	maxBadgeLabel     = 40 // runes, longer monitor names are cut
)

// badgeRanges are the uptime windows a badge accepts, in days.
var badgeRanges = map[string]int{"24h": 1, "7d": 7, "30d": 30, "90d": 90}

// StatusBadge serves GET /badge/status.svg. Without ?monitor it shows the
// overall status of the page that matches the request Host. With ?monitor it
// shows the status of that one monitor, which must be on the page.
func (p *Public) StatusBadge(w http.ResponseWriter, r *http.Request) {
	mons, mon, ok := p.badgeScope(w, r)
	if !ok {
		return
	}
	if mon == nil {
		anyDown, anyDegraded := false, false
		for _, m := range mons.list {
			switch mons.statuses[m.ID] {
			case "down":
				anyDown = true
			case "degraded":
				anyDegraded = true
			}
		}
		switch {
		case anyDown:
			writeBadge(w, "status", "outage", badgeRed)
		case anyDegraded:
			writeBadge(w, "status", "degraded", badgeYellow)
		default:
			writeBadge(w, "status", "operational", badgeGreen)
		}
		return
	}
	text, color := "no data", badgeGrey
	switch mons.statuses[mon.ID] {
	case "up":
		text, color = "up", badgeGreen
	case "degraded":
		text, color = "degraded", badgeYellow
	case "down":
		text, color = "down", badgeRed
	}
	writeBadge(w, mon.Name, text, color)
}

// UptimeBadge serves GET /badge/uptime.svg?range=24h|7d|30d|90d. Without
// ?monitor it shows the mean uptime of the page's monitors that have checks
// in the range. With ?monitor it shows that one monitor's uptime.
func (p *Public) UptimeBadge(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = defaultBadgeRange
	}
	days, valid := badgeRanges[rng]
	if !valid {
		http.Error(w, "range must be one of 24h, 7d, 30d, 90d", http.StatusBadRequest)
		return
	}
	mons, mon, ok := p.badgeScope(w, r)
	if !ok {
		return
	}
	label := "uptime " + rng
	list := mons.list
	if mon != nil {
		label = mon.Name + " " + label
		list = []generated.Monitor{*mon}
	}
	since := time.Now().AddDate(0, 0, -days).UTC()
	var sum float64
	var n int
	for _, m := range list {
		pct, has, err := p.uptime(r.Context(), m.ID, since)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		if has {
			sum += pct
			n++
		}
	}
	if n == 0 {
		writeBadge(w, label, "no data", badgeGrey)
		return
	}
	pct := sum / float64(n)
	color := badgeRed
	switch {
	case pct >= 99:
		color = badgeGreen
	case pct >= 95:
		color = badgeYellow
	}
	writeBadge(w, label, formatPct(pct), color)
}

// uptime returns a monitor's uptime percentage since a time, and whether it
// has any checks in that window at all.
func (p *Public) uptime(ctx context.Context, monitorID int64, since time.Time) (float64, bool, error) {
	rows, err := p.q.ListCheckResults(ctx, generated.ListCheckResultsParams{MonitorID: monitorID, CheckedAt: since, Limit: 1})
	if err != nil || len(rows) == 0 {
		return 0, false, err
	}
	pct, err := p.q.UptimePercent(ctx, generated.UptimePercentParams{MonitorID: monitorID, CheckedAt: since})
	return pct, err == nil, err
}

type pageMonitors struct {
	list     []generated.Monitor
	statuses map[int64]string
}

// badgeScope resolves the page for the request Host and the monitors it
// shows, the same way /api/status does. When ?monitor is set it also picks
// that monitor by name, or by ID, among the page's monitors only. It writes
// a 404 and returns false for an unpublished page or a monitor the page does
// not show, so a badge never confirms that a hidden monitor exists.
func (p *Public) badgeScope(w http.ResponseWriter, r *http.Request) (pageMonitors, *generated.Monitor, bool) {
	rp, serve := ResolvePage(r.Context(), p.q, r.Host)
	if !serve {
		http.NotFound(w, r)
		return pageMonitors{}, nil, false
	}
	snap, err := handlers.Snapshot(r.Context(), p.q)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return pageMonitors{}, nil, false
	}
	allow := make(map[int64]bool, len(rp.GroupIDs))
	for _, id := range rp.GroupIDs {
		allow[id] = true
	}
	pm := pageMonitors{statuses: snap.Statuses}
	for _, m := range snap.Monitors {
		if m.IsActive && m.GroupID != nil && (rp.GroupIDs == nil || allow[*m.GroupID]) {
			pm.list = append(pm.list, m)
		}
	}
	want := strings.TrimSpace(r.URL.Query().Get("monitor"))
	if want == "" {
		return pm, nil, true
	}
	if m := findMonitor(pm.list, want); m != nil {
		return pm, m, true
	}
	http.NotFound(w, r)
	return pageMonitors{}, nil, false
}

// findMonitor matches a name first, ignoring case, then a numeric ID.
func findMonitor(list []generated.Monitor, want string) *generated.Monitor {
	for i := range list {
		if strings.EqualFold(list[i].Name, want) {
			return &list[i]
		}
	}
	if id, err := strconv.ParseInt(want, 10, 64); err == nil {
		for i := range list {
			if list[i].ID == id {
				return &list[i]
			}
		}
	}
	return nil
}

// formatPct keeps two decimals but drops trailing zeros: 100%, 99.5%, 99.95%.
func formatPct(pct float64) string {
	s := strconv.FormatFloat(pct, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + "%"
}

// badgeTextWidth estimates the rendered width of 11px Verdana text.
func badgeTextWidth(s string) int {
	return len([]rune(s))*7 + 10
}

// writeBadge renders a shields-style flat SVG. Both strings are escaped,
// since a monitor name is user input.
func writeBadge(w http.ResponseWriter, label, message, color string) {
	if r := []rune(label); len(r) > maxBadgeLabel {
		label = string(r[:maxBadgeLabel-1]) + "…"
	}
	lw, mw := badgeTextWidth(label), badgeTextWidth(message)
	total := lw + mw
	l, m := html.EscapeString(label), html.EscapeString(message)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%[1]d" height="20" role="img" aria-label="%[4]s: %[5]s">`+
		`<title>%[4]s: %[5]s</title>`+
		`<linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>`+
		`<clipPath id="r"><rect width="%[1]d" height="20" rx="3" fill="#fff"/></clipPath>`+
		`<g clip-path="url(#r)"><rect width="%[2]d" height="20" fill="%[6]s"/><rect x="%[2]d" width="%[3]d" height="20" fill="%[7]s"/><rect width="%[1]d" height="20" fill="url(#s)"/></g>`+
		`<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">`+
		`<text x="%[8]d" y="15" fill="#010101" fill-opacity=".3">%[4]s</text><text x="%[8]d" y="14">%[4]s</text>`+
		`<text x="%[9]d" y="15" fill="#010101" fill-opacity=".3">%[5]s</text><text x="%[9]d" y="14">%[5]s</text>`+
		`</g></svg>`,
		total, lw, mw, l, m, badgeLabel, color, lw/2, lw+mw/2)

	h := w.Header()
	h.Set("Content-Type", "image/svg+xml; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=60")
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write([]byte(svg)); err != nil {
		log.Printf("web: write badge: %v", err)
	}
}
