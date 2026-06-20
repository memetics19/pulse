package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
)

var (
	mdBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdItalic = regexp.MustCompile(`\*([^*]+)\*`)
	mdCode   = regexp.MustCompile("`([^`]+)`")
	mdLink   = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
)

// renderMarkdown renders a small, safe subset of markdown for admin-authored
// incident messages: the input is HTML-escaped first, then bold/italic/code/
// links and line breaks are applied.
func renderMarkdown(s string) template.HTML {
	e := template.HTMLEscapeString(s)
	e = mdLink.ReplaceAllString(e, `<a href="$2" target="_blank" rel="noopener">$1</a>`)
	e = mdBold.ReplaceAllString(e, "<strong>$1</strong>")
	e = mdItalic.ReplaceAllString(e, "<em>$1</em>")
	e = mdCode.ReplaceAllString(e, "<code>$1</code>")
	e = strings.ReplaceAll(e, "\n", "<br>")
	return template.HTML(e)
}

//go:embed templates/status.html
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

var statusTmpl = template.Must(template.ParseFS(templateFiles, "templates/status.html"))

type Public struct{ q *generated.Queries }

func NewPublic(q *generated.Queries) *Public { return &Public{q: q} }

type barVM struct{ Class, Title string }

func barStatusText(class string) string {
	switch class {
	case "down":
		return "Down"
	case "degraded":
		return "Degraded"
	case "nodata":
		return "No data"
	default:
		return "Operational"
	}
}

type monitorVM struct {
	Name        string
	StatusClass string
	UptimePct   string
	AvgMs       string
	Bars        []barVM
}

type groupVM struct {
	Name     string
	Monitors []monitorVM
}

type incUpdateVM struct {
	Status, When string
	Message      template.HTML
}

type incidentVM struct {
	Title, Status, Severity string
	Updates                 []incUpdateVM
}

type maintVM struct {
	Title            string
	Description      template.HTML
	StartISO, EndISO string   // RFC3339, rendered via .localtime
	Affected         []string // monitor names
}

type rangeOpt struct {
	Key, Label string
	Active     bool
}

type footerLinkVM struct {
	Label string
	URL   template.URL
}

type footerVM struct {
	BrandLine, Tagline, Copyright string
	HidePoweredBy                 bool
	Links                         []footerLinkVM
}

type statusVM struct {
	SiteName, ThemePreset          string
	LogoURL, FaviconURL            template.URL
	CustomCSS                      template.CSS
	OverallText, OverallClass      string
	Ranges                         []rangeOpt
	Groups                         []groupVM
	ActiveIncidents, PastIncidents []incidentVM
	InProgressMaint, UpcomingMaint []maintVM
	Footer                         footerVM
}

const pastIncidentLimit = 10

type rangeSpec struct{ days, buckets int }

var rangeSpecs = map[string]rangeSpec{
	"90d": {90, 90}, "60d": {60, 60}, "30d": {30, 45},
	"15d": {15, 45}, "7d": {7, 42}, "24h": {1, 48},
}

var rangeOrder = []struct{ Key, Label string }{
	{"90d", "90d"}, {"60d", "60d"}, {"30d", "30d"}, {"15d", "15d"}, {"7d", "1w"}, {"24h", "Today"},
}

func (p *Public) Get(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if _, ok := rangeSpecs[rng]; !ok {
		rng = "90d"
	}
	rp, serve := ResolvePage(r.Context(), p.q, r.Host)
	if !serve {
		http.NotFound(w, r)
		return
	}
	vm := p.buildVM(r.Context(), rng, rp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := statusTmpl.Execute(w, vm); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// affectsPage reports whether an incident/maintenance window (described by its
// affected_monitor_ids JSON) is relevant to the current page. The default page
// (allGroups) shows everything; a scoped page shows only items touching one of
// the monitors it renders.
func affectsPage(affectedJSON string, pageMonitors map[int64]bool, allGroups bool) bool {
	if allGroups {
		return true
	}
	var ids []int64
	if affectedJSON != "" {
		_ = json.Unmarshal([]byte(affectedJSON), &ids)
	}
	for _, id := range ids {
		if pageMonitors[id] {
			return true
		}
	}
	return false
}

func (p *Public) buildVM(ctx context.Context, rng string, rp ResolvedPage) statusVM {
	snap := handlers.Snapshot(ctx, p.q)
	spec := rangeSpecs[rng]
	now := time.Now()
	since := now.AddDate(0, 0, -spec.days)

	cfg := struct {
		SiteName   string `json:"site_name"`
		LogoURL    string `json:"logo_url"`
		FaviconURL string `json:"favicon_url"`
		Footer     struct {
			BrandLine     string `json:"brand_line"`
			Tagline       string `json:"tagline"`
			Copyright     string `json:"copyright"`
			HidePoweredBy bool   `json:"hide_powered_by"`
			Links         []struct {
				Label string `json:"label"`
				URL   string `json:"url"`
			} `json:"links"`
		} `json:"footer"`
	}{SiteName: "Status"}
	if snap.Theme.ConfigJson != "" {
		_ = json.Unmarshal([]byte(snap.Theme.ConfigJson), &cfg)
	}
	if cfg.SiteName == "" {
		cfg.SiteName = "Status"
	}
	// A named (non-default) page wins over the configured site name; the default
	// page keeps the theme's site_name.
	if rp.Page.Title != "" && rp.Page.Title != "Status" {
		cfg.SiteName = rp.Page.Title
	}

	footer := footerVM{
		BrandLine:     cfg.Footer.BrandLine,
		Tagline:       cfg.Footer.Tagline,
		Copyright:     cfg.Footer.Copyright,
		HidePoweredBy: cfg.Footer.HidePoweredBy,
	}
	for _, l := range cfg.Footer.Links {
		footer.Links = append(footer.Links, footerLinkVM{Label: l.Label, URL: template.URL(l.URL)})
	}

	vm := statusVM{
		SiteName:    cfg.SiteName,
		LogoURL:     template.URL(cfg.LogoURL),
		FaviconURL:  template.URL(cfg.FaviconURL),
		CustomCSS:   template.CSS(snap.Theme.CustomCss),
		ThemePreset: snap.Theme.Preset,
		Footer:      footer,
	}
	for _, o := range rangeOrder {
		vm.Ranges = append(vm.Ranges, rangeOpt{Key: o.Key, Label: o.Label, Active: o.Key == rng})
	}

	const (
		statusUp       = "up"
		statusDown     = "down"
		statusDegraded = "degraded"
	)
	anyDown, anyDegraded := false, false

	// allGroups is the default page (shows every group); a scoped page restricts
	// to the groups in rp.GroupIDs.
	allGroups := rp.GroupIDs == nil
	allowSet := make(map[int64]bool, len(rp.GroupIDs))
	for _, id := range rp.GroupIDs {
		allowSet[id] = true
	}
	allowGroup := func(id int64) bool {
		if allGroups {
			return true
		}
		return allowSet[id]
	}

	groups := append([]generated.MonitorGroup(nil), snap.Groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].DisplayOrder < groups[j].DisplayOrder })

	// pageMonitors collects the IDs of the active monitors actually shown on this
	// page; it scopes which incidents/maintenance windows are relevant.
	pageMonitors := map[int64]bool{}
	byGroup := make(map[int64][]monitorVM)
	for _, m := range snap.Monitors {
		if !m.IsActive || m.GroupID == nil {
			continue
		}
		if !allowGroup(*m.GroupID) {
			continue
		}
		pageMonitors[m.ID] = true
		st := snap.Statuses[m.ID]
		if st == "" {
			st = statusUp
		}
		switch st {
		case statusDown:
			anyDown = true
		case statusDegraded:
			anyDegraded = true
		}
		mvm := monitorVM{Name: m.Name, StatusClass: st}
		mvm.Bars, mvm.UptimePct, mvm.AvgMs = p.monitorSeries(ctx, m.ID, since, now, spec.buckets)
		byGroup[*m.GroupID] = append(byGroup[*m.GroupID], mvm)
	}

	for _, g := range groups {
		if !allowGroup(g.ID) {
			continue
		}
		vm.Groups = append(vm.Groups, groupVM{Name: g.Name, Monitors: byGroup[g.ID]})
	}

	switch {
	case anyDown:
		vm.OverallText, vm.OverallClass = "Service disruption", "outage"
	case anyDegraded:
		vm.OverallText, vm.OverallClass = "Degraded performance", "degraded"
	default:
		vm.OverallText, vm.OverallClass = "All systems operational", ""
	}

	for _, inc := range snap.Incidents {
		if !affectsPage(inc.AffectedMonitorIds, pageMonitors, allGroups) {
			continue
		}
		ivm := incidentVM{Title: inc.Title, Status: inc.Status, Severity: inc.Severity}
		if ups, err := p.q.ListIncidentUpdates(ctx, inc.ID); err == nil {
			for _, u := range ups {
				ivm.Updates = append(ivm.Updates, incUpdateVM{
					Status: u.Status, Message: renderMarkdown(u.Message), When: u.CreatedAt.UTC().Format(time.RFC3339),
				})
			}
		}
		if inc.Status == "resolved" {
			if len(vm.PastIncidents) < pastIncidentLimit {
				vm.PastIncidents = append(vm.PastIncidents, ivm)
			}
		} else {
			vm.ActiveIncidents = append(vm.ActiveIncidents, ivm)
		}
	}

	monitorName := map[int64]string{}
	for _, m := range snap.Monitors {
		monitorName[m.ID] = m.Name
	}
	if windows, err := p.q.ListMaintenance(ctx); err == nil {
		for _, win := range windows {
			if win.Status != "in_progress" && win.Status != "scheduled" {
				continue
			}
			if !affectsPage(win.AffectedMonitorIds, pageMonitors, allGroups) {
				continue
			}
			var ids []int64
			if win.AffectedMonitorIds != "" {
				_ = json.Unmarshal([]byte(win.AffectedMonitorIds), &ids)
			}
			var names []string
			for _, id := range ids {
				if n, ok := monitorName[id]; ok {
					names = append(names, n)
				}
			}
			mv := maintVM{
				Title:       win.Title,
				Description: renderMarkdown(win.Description),
				StartISO:    win.StartsAt.UTC().Format(time.RFC3339),
				EndISO:      win.EndsAt.UTC().Format(time.RFC3339),
				Affected:    names,
			}
			if win.Status == "in_progress" {
				vm.InProgressMaint = append(vm.InProgressMaint, mv)
			} else {
				vm.UpcomingMaint = append(vm.UpcomingMaint, mv)
			}
		}
	}

	return vm
}

// monitorSeries builds uptime bars + uptime% + avg latency for one monitor over [since, now].
func (p *Public) monitorSeries(ctx context.Context, monitorID int64, since, now time.Time, buckets int) ([]barVM, string, string) {
	rows, _ := p.q.ListCheckResults(ctx, generated.ListCheckResultsParams{
		MonitorID: monitorID, CheckedAt: since, Limit: 10000,
	})

	worst := make([]string, buckets) // "" = nodata
	window := now.Sub(since)
	if window <= 0 {
		window = time.Hour
	}
	rank := map[string]int{"up": 1, "degraded": 2, "down": 3}
	var sumMs, nMs, upCount, total int64
	for _, cr := range rows {
		total++
		if cr.Status == "up" {
			upCount++
		}
		if cr.ResponseTimeMs != nil {
			sumMs += *cr.ResponseTimeMs
			nMs++
		}
		idx := int(float64(cr.CheckedAt.Sub(since)) / float64(window) * float64(buckets))
		if idx < 0 {
			idx = 0
		}
		if idx >= buckets {
			idx = buckets - 1
		}
		if rank[cr.Status] > rank[worst[idx]] {
			worst[idx] = cr.Status
		}
	}

	bucketDur := window / time.Duration(buckets)
	bars := make([]barVM, buckets)
	for i, s := range worst {
		if s == "" {
			s = "nodata"
		}
		label := since.Add(time.Duration(i) * bucketDur).Format("Jan 2")
		bars[i] = barVM{Class: s, Title: label + " · " + barStatusText(s)}
	}

	uptime := "—"
	if total > 0 {
		uptime = fmt.Sprintf("%.2f%%", float64(upCount)/float64(total)*100)
	}
	avg := "—"
	if nMs > 0 {
		avg = fmt.Sprintf("%d ms", sumMs/nMs)
	}
	return bars, uptime, avg
}

// StaticHandler serves /static/* (public css + js) from the embedded FS.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
