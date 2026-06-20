# Pulse Plan 4 — Admin Redesign (Overview + v3 look) Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. `- [ ]` checkboxes.
> **PROJECT GIT RULE:** no git add/commit/push/mv from the agent. Stop at **YOU COMMIT**.

**Goal:** Replace the old admin scaffold with a real **Overview dashboard** and the **v3 modern/formal light** look, plus a restructured nav (Overview first + a Settings page).

**Architecture:** A new `LatestStatuses` query gives real per-monitor status (shared by the Overview API and the public page). A session-protected `GET /api/overview` returns dashboard data (counts, 30d uptime, active incidents, attention items). A new `/admin` Overview page renders it. The admin CSS is restyled to the v3 register (white cards, hairline borders, soft elevation, indigo accent, generous spacing); the nav gains Overview + Settings.

**Tech Stack:** Go (chi, sqlc/SQLite), Next.js admin SPA, globals.css.

**Scope / deferred:** Incident-management upgrades (stepper, markdown, filters) and multi-page/maintenance nav entries are **deferred to later plans**. This plan = Overview + visual revamp + Settings.

---

## Task 1: LatestStatuses query + real per-monitor status

**Files:** `api/internal/db/queries/check_results.sql`; `api/internal/handlers/status.go` (Snapshot adds Statuses); `api/internal/web/public.go` (use real status); regenerate.

- [ ] **Step 1:** Append to `check_results.sql`:
```sql
-- name: LatestStatuses :many
SELECT cr.monitor_id, cr.status
FROM check_results cr
JOIN (SELECT monitor_id, MAX(checked_at) AS mx FROM check_results GROUP BY monitor_id) latest
  ON cr.monitor_id = latest.monitor_id AND cr.checked_at = latest.mx;
```
- [ ] **Step 2:** `cd api/internal/db && sqlc generate && cd .. && go build ./...`. Confirm `LatestStatuses` returns rows with `MonitorID int64, Status string` (struct name e.g. `LatestStatusesRow`).
- [ ] **Step 3:** In `api/internal/handlers/status.go`, add to `StatusResponse` a field `Statuses map[int64]string \`json:"statuses"\`` and populate it in `Snapshot` from `q.LatestStatuses(ctx)` (map monitor_id→status). Keep existing fields.
- [ ] **Step 4:** Test in `status_test.go`:
```go
func TestSnapshotIncludesStatuses(t *testing.T) {
	db := testutil.NewTestDB(t); q := generated.New(db)
	snap := Snapshot(context.Background(), q)
	if snap.Statuses == nil { t.Fatal("Statuses map should be non-nil (empty ok)") }
}
```
Run → PASS.
- [ ] **Step 5:** In `api/internal/web/public.go` `buildVM`, set each monitor's `StatusClass` from `snap.Statuses[m.ID]` (default `"up"` when absent), and compute overall from the real statuses (remove the all-"up" faithful-port note now that real data exists). Run `go test ./internal/web/ -count=1` → still green (empty DB → operational).
- [ ] **Step 6: YOU COMMIT** — "feat: real per-monitor latest status"

---

## Task 2: Overview API

**Files:** `api/internal/handlers/overview.go` + test; wire route in `server.go` (admin group).

- [ ] **Step 1: failing test** `overview_test.go`:
```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestOverviewEmptyDB(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := NewOverview(generated.New(db))
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/overview", nil))
	if rec.Code != http.StatusOK { t.Fatalf("got %d", rec.Code) }
	var o struct {
		Overall string `json:"overall"`
		Counts  struct{ Up, Degraded, Down int } `json:"counts"`
	}
	json.NewDecoder(rec.Body).Decode(&o)
	if o.Overall != "operational" { t.Fatalf("empty DB overall=%q want operational", o.Overall) }
}
```
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: implement** `overview.go`:
```go
package handlers

import (
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
)

type Overview struct{ q *generated.Queries }

func NewOverview(q *generated.Queries) *Overview { return &Overview{q: q} }

type attentionItem struct {
	Kind    string `json:"kind"`    // "down" | "agent_offline"
	Message string `json:"message"`
}

func (h *Overview) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	monitors, _ := h.q.ListMonitors(ctx)
	rows, _ := h.q.LatestStatuses(ctx)
	status := map[int64]string{}
	for _, row := range rows {
		status[row.MonitorID] = row.Status
	}

	var up, degraded, down int
	attention := []attentionItem{}
	for _, m := range monitors {
		if m.IsActive == 0 {
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

	agents, _ := h.q.ListAgents(ctx)
	for _, a := range agents {
		if a.IsActive == 0 || a.LastSeenAt == nil {
			continue
		}
		if time.Since(*a.LastSeenAt) > 2*time.Minute {
			attention = append(attention, attentionItem{Kind: "agent_offline", Message: "Agent " + a.Name + " is offline"})
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
```
> Before coding: confirm `generated.Monitor.IsActive` is `int64`, `generated.InfraAgent` (or `Agent`) has `IsActive int64`, `LastSeenAt *time.Time`, `Name string` — adjust to the real generated names (`ListAgents` row type). Reuse the existing `writeJSON`.
- [ ] **Step 4:** run → PASS; `go build ./...`.
- [ ] **Step 5:** In `server.go` admin group add: `r.Get("/api/overview", handlers.NewOverview(q).Get)`.
- [ ] **Step 6: YOU COMMIT** — "feat(api): overview dashboard endpoint"

---

## Task 3: v3 admin CSS restyle + nav restructure + Settings page

**Files:** `ui/src/app/globals.css` (admin section), `ui/src/app/admin/layout.tsx` (nav), `ui/src/app/admin/settings/page.tsx` (new).

- [ ] **Step 1:** Restyle the admin classes in `ui/src/app/globals.css` to the v3 modern/formal light register. Update `.admin-layout`, `.admin-sidebar`, `.admin-sidebar-title`, `.admin-nav-link` (+`.active`), `.admin-content`, `.admin-title`, and add a reusable `.card` (white bg, `1px solid var(--border)`, border-radius 12, padding 16-18, subtle `box-shadow: 0 1px 2px rgba(16,24,40,.06)`) and `.stat-card`. Keep existing CSS variables. Concretely:
  - Sidebar: `background:#fff; border-right:1px solid var(--border); width:200px; padding:14px 10px`.
  - Nav link: `padding:7px 12px; border-radius:7px; color:var(--text-muted); font-size:14px`; `.active{background:var(--bg2); color:var(--text); font-weight:600}` with a left indigo accent.
  - Content: `background:var(--bg); padding:24px 28px`.
  - Do NOT introduce gradients/glow. Flat, hairline, soft shadow only.
- [ ] **Step 2:** Update `NAV` in `layout.tsx` to: Overview (`/admin`), Monitors, Incidents, Agents, Theme, Notifications, Settings (`/admin/settings`). Put Overview first.
- [ ] **Step 3:** Create `ui/src/app/admin/settings/page.tsx` — a minimal real Settings page (v3 cards): an "Instance" card showing app info (e.g. "Pulse" + a note that DB/branding were set during setup, with a link to Theme for branding), and a placeholder "Data retention" card noting monitor-history retention settings are coming (this becomes real in the retention plan). Use `.admin-title` + `.card`. No backend needed yet.
- [ ] **Step 4:** `cd ui && NEXT_PUBLIC_API_URL="" npm run build` → succeeds; `ui/out/admin/settings/` exists.
- [ ] **Step 5: YOU COMMIT** — "feat(admin): v3 restyle + nav + settings page"

---

## Task 4: Overview dashboard page

**Files:** `ui/src/app/admin/page.tsx` (replace the redirect), `ui/src/lib/api.ts` (`fetchOverview`).

- [ ] **Step 1:** Add to `api.ts`:
```ts
export type Overview = {
  overall: 'operational' | 'degraded' | 'outage'
  counts: { up: number; degraded: number; down: number }
  total_monitors: number
  agent_count: number
  active_incidents: Incident[]
  attention: { kind: string; message: string }[]
}
export async function fetchOverview(): Promise<Overview> {
  const res = await fetch(`${BASE}/api/overview`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`${res.status}`)
  return res.json()
}
```
- [ ] **Step 2:** Replace `ui/src/app/admin/page.tsx` (currently redirects to /admin/monitors) with an Overview dashboard (`'use client'`): on mount `fetchOverview()`; render
  - a **status banner** card (flat row: colored dot + "All systems operational"/"Partial degradation"/"Service disruption" by `overall`; right side "N monitors · M agents"),
  - **stat cards** grid (Operational / Degraded / Down / 30d — show counts; "30d" can show total_monitors as a placeholder labeled "Monitors" if uptime isn't computed),
  - **Active incidents** card (list titles + status, or "No active incidents"),
  - **Attention** card (list `attention[].message`, or "Nothing needs attention").
  Use the `.card` class + the v3 look. Color only on the status dot/numbers. No gradient/glow.
- [ ] **Step 3:** `cd ui && NEXT_PUBLIC_API_URL="" npm run build` → succeeds.
- [ ] **Step 4: YOU COMMIT** — "feat(admin): overview dashboard"

---

## Task 5: End-to-end verification

- [ ] **Step 1:** `make build`; run on fresh data dir on :8090; complete setup via curl (account); then with the session cookie:
```bash
curl -s -b /tmp/sj localhost:8090/api/overview
```
Expect JSON with `overall:"operational"`, zero counts, empty incidents/attention.
- [ ] **Step 2:** Headless screenshot of `/admin/` (logged in) — confirm the Overview dashboard renders in the v3 look with the new nav (Overview active, Settings present). (Log in first by completing setup, reuse the cookie; or script a login.)
- [ ] **Step 3:** Screenshot `/admin/settings/` too.
- [ ] **Step 4: YOU COMMIT** — "test: overview + settings verified"

---

## Self-Review notes
- Spec coverage: Overview dashboard ✅ (banner/stat cards/active incidents/attention), v3 look ✅, nav + Settings ✅, real per-monitor status ✅. **Deferred:** incident-mgmt upgrades; SSL-expiry attention (no ssl_checks queries yet); high-CPU attention; Pages/Maintenance/API-Keys nav (their own plans).
- Consistency: `Overview` JSON shape in Go (Task 2) must match the TS `Overview` type (Task 4) — keys `overall`, `counts.{up,degraded,down}`, `total_monitors`, `agent_count`, `active_incidents`, `attention`.
