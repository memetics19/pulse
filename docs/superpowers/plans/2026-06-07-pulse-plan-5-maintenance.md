# Pulse Plan 5 — Maintenance Windows Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. `- [ ]` checkboxes.
> **PROJECT GIT RULE:** no git add/commit/push/mv from the agent. Stop at **YOU COMMIT**.

**Goal:** Scheduled (and start-now) maintenance windows that auto-transition through their lifecycle, suppress alerts/auto-incidents for affected monitors while active, and show on the public page (in-progress banner · upcoming · completed-in-history).

**Architecture:** A `maintenance_windows` table (status scheduled|in_progress|completed|cancelled, starts_at, ends_at, affected_monitor_ids JSON). Admin CRUD under `/api/maintenance` (session-or-API-key, `maintenance:*` scope). The in-process worker gains a ticker that flips `scheduled→in_progress` once `starts_at` passes and `→completed` once `ends_at` passes. The incident detector skips monitors covered by an active window (alert suppression). The Go public page renders an in-progress banner (monochrome wrench), an "Upcoming maintenance" section, and completed windows in history. An admin **Maintenance** page lists/creates/cancels windows.

**Scope / deferred:** per-window public update timeline and email/Slack notifications are **deferred** (note in UI). Auto-scoping to specific status pages is moot until Plan 3 (multi-page) — windows are global for now.

**Builds on:** Plan 2c (`app`/server wiring), worker (`worker.Run` loop), incident detector, Go public template, API-key scopes.

---

## Task 1: schema + queries
**Files:** `api/internal/db/migrations/6_maintenance.{up,down}.sql`, `api/internal/db/queries/maintenance.sql`, regenerate.

- [ ] **Step 1** `6_maintenance.up.sql`:
```sql
CREATE TABLE maintenance_windows (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    title                TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'scheduled',
    affected_monitor_ids TEXT NOT NULL DEFAULT '[]',
    starts_at            TIMESTAMP NOT NULL,
    ends_at              TIMESTAMP NOT NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```
`6_maintenance.down.sql`: `DROP TABLE maintenance_windows;`
- [ ] **Step 2** `maintenance.sql`:
```sql
-- name: CreateMaintenance :one
INSERT INTO maintenance_windows (title, description, status, affected_monitor_ids, starts_at, ends_at)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ListMaintenance :many
SELECT * FROM maintenance_windows ORDER BY starts_at DESC;

-- name: ListActiveMaintenance :many
SELECT * FROM maintenance_windows WHERE status = 'in_progress';

-- name: UpdateMaintenanceStatus :exec
UPDATE maintenance_windows SET status = ? WHERE id = ?;

-- name: DeleteMaintenance :exec
DELETE FROM maintenance_windows WHERE id = ?;

-- name: DueToStart :many
SELECT * FROM maintenance_windows WHERE status = 'scheduled' AND starts_at <= ?;

-- name: DueToEnd :many
SELECT * FROM maintenance_windows WHERE status = 'in_progress' AND ends_at <= ?;
```
- [ ] **Step 3** regenerate + build. Note generated `MaintenanceWindow` fields + `CreateMaintenanceParams`.
- [ ] **Step 4** migration smoke test (insert + select) → PASS.
- [ ] **Step 5: YOU COMMIT** — "feat(db): maintenance_windows schema + queries"

---

## Task 2: CRUD handlers + routes
**Files:** `api/internal/handlers/maintenance.go` + test; `server.go` (add routes to the session-or-API-key group).

- [ ] **Step 1: failing test** — `NewMaintenance(q)`; Create (POST `{title, description, affected_monitor_ids:[], starts_at, ends_at, start_now?}`) → 201; List → includes it; Delete → 204. (start_now true ⇒ status `in_progress` and starts_at=now; else `scheduled`.)
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3** implement `maintenance.go`: `NewMaintenance(q)`, `List`, `Create`, `UpdateStatus` (e.g. cancel), `Delete`. Parse RFC3339 `starts_at`/`ends_at` from JSON; `affected_monitor_ids` stored as JSON string. `start_now` sets status `in_progress`, `starts_at=now`. Reuse `writeJSON`. Marshal/parse affected ids like the incidents handler does.
- [ ] **Step 4** run → PASS; build.
- [ ] **Step 5** `server.go`: in the `RequireSessionOrAPIKey` group add:
```go
r.Route("/api/maintenance", func(r chi.Router) {
    h := handlers.NewMaintenance(q)
    r.Get("/", h.List)
    r.Post("/", h.Create)
    r.Put("/{id}/status", h.UpdateStatus)
    r.Delete("/{id}", h.Delete)
})
```
Also add `maintenance` to the `requiredScope` map in `middleware/apikey.go` (`/api/maintenance` → `maintenance:read|write`) and to the API-key scope checklist in the admin UI (Plan 2b page) — and to the create-incident affected-monitor multiselect reuse if helpful.
- [ ] **Step 6: YOU COMMIT** — "feat(api): maintenance CRUD"

---

## Task 3: worker auto-transitions
**Files:** `api/internal/worker/maintenance/maintenance.go` (+ test); `api/internal/worker/worker.go` (call it in the existing 1-minute ticker loop).

- [ ] **Step 1: failing test**: seed a `scheduled` window with `starts_at` in the past → `Run(ctx, q, now)` flips it to `in_progress`; an `in_progress` with `ends_at` in the past → `completed`.
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3** implement a `Sweeper` with `Run(ctx, now)` that calls `DueToStart(now)` → `UpdateMaintenanceStatus("in_progress")` each; `DueToEnd(now)` → `completed`. (Operates on `store.Queries`.)
- [ ] **Step 4** wire into `worker.go`: in the existing `tick` loop (next to rollup/pruner), call the maintenance sweeper with `time.Now()`.
- [ ] **Step 5** run → PASS; build. **Step 6: YOU COMMIT** — "feat(worker): maintenance auto-transitions"

---

## Task 4: alert suppression
**Files:** `api/internal/worker/incident/detector.go` (+ test).

- [ ] **Step 1: failing test**: with an active maintenance window covering monitor X, `MaybeCreateIncident(ctx, X, "down")` (after the consecutive-down threshold) must NOT create an incident.
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3** implement: before creating the auto-incident, call `ListActiveMaintenance(ctx)` and if any active window's `affected_monitor_ids` contains `monitorID`, return `(false, nil)` (suppressed). Reuse the existing `containsMonitorID` helper.
- [ ] **Step 4** run → PASS; build. **Step 5: YOU COMMIT** — "feat(worker): suppress alerts during maintenance"

---

## Task 5: public page maintenance display
**Files:** `api/internal/web/public.go`, `templates/status.html`, `static/public.css`.

- [ ] **Step 1** In `buildVM`, load maintenance via `p.q.ListMaintenance(ctx)`; build:
  - `InProgressMaintenance []maintVM` (status in_progress),
  - `UpcomingMaintenance []maintVM` (status scheduled),
  - completed windows folded into the existing history section (or a `MaintenanceHistory`).
  `maintVM{Title, Description (template.HTML via renderMarkdown), Window string (local-time range via RFC3339 + `.localtime`), AffectedNames []string}`.
- [ ] **Step 2** Template: an **in-progress banner** above the hero/services — a card with a monochrome wrench SVG + accent left-bar (NO emoji) showing title + affected + "until <time>". An **"Upcoming maintenance"** section (scheduled windows, local-time range). Completed windows appear in history.
- [ ] **Step 3** CSS: `.maint-banner` (indigo-tinted card + left accent bar), `.maint-card`, reuse `.localtime` for times. Flat, no glow.
- [ ] **Step 4** build + `go test ./internal/web/` → green (empty DB → no maintenance sections). **Step 5: YOU COMMIT** — "feat(public): maintenance banner + upcoming"

---

## Task 6: admin Maintenance page + nav
**Files:** `ui/src/app/admin/maintenance/page.tsx` (new), `ui/src/app/admin/layout.tsx` (nav), `ui/src/lib/api.ts`.

- [ ] **Step 1** `api.ts`: `listMaintenance()`, `createMaintenance(payload)`, `updateMaintenanceStatus(id, status)`, `deleteMaintenance(id)` (cookie).
- [ ] **Step 2** `layout.tsx` NAV: add `{ href: '/admin/maintenance', label: 'Maintenance' }` (after Incidents).
- [ ] **Step 3** `maintenance/page.tsx` (v3 look): list windows (title, status chip, local-time range, affected count) with Cancel/Delete (inline confirm); a "New maintenance" modal with title, description, affected-monitor multiselect (from `adminListMonitors`), start/end `datetime-local` inputs, and a "Start now" checkbox. Submit → `createMaintenance` (send `starts_at`/`ends_at` as RFC3339; `start_now` bool).
- [ ] **Step 4** build → succeeds; `ui/out/admin/maintenance/` present. **Step 5: YOU COMMIT** — "feat(admin): maintenance page"

---

## Self-Review
- Spec coverage: scheduled+start-now ✅, auto-transition ✅, alert suppression ✅, public banner/upcoming/history ✅, admin CRUD ✅. Deferred (noted): per-window update timeline, notifications.
- Consistency: `affected_monitor_ids` JSON string handling matches the incidents handler; times are RFC3339 + client-local (`.localtime`) like incident updates; `maintenance:read|write` scope added to both the apikey middleware map and the API-key UI checklist.
