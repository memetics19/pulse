# Pulse Plan 3 (v1) — Multiple Status Pages (core routing + per-page content)

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. `- [ ]` checkboxes.
> **PROJECT GIT RULE:** no git add/commit/push/mv from the agent. Stop at **YOU COMMIT**.

**Goal:** Host more than one public status page. Each page has a title, a custom domain, a published flag, and a chosen set of monitor groups. The public page is resolved by the request `Host` header; an unknown host falls back to the **default** page (which shows all groups, preserving today's behaviour). An admin **Pages** section manages them.

**Architecture:** New `status_pages` (+ `status_page_groups`) tables; a migration seeds one `is_default` page. A resolver maps `Host` → page (or the default). The Go public renderer resolves the page, uses its title, and shows only its groups' monitors — with incidents/maintenance scoped to those monitors. Non-default unpublished pages 404. Admin Pages CRUD + UI.

**Scope / deferred (later passes):** autocert per-domain TLS (host routing works now behind any proxy / via the existing setup); **per-page branding/theme/footer** (branding stays the global Theme config for all pages in v1); per-page **RSS**. These are explicitly out of this pass.

**Builds on:** Plan 2c (app/server), the Go public renderer (`web/public.go`), groups/monitors, API-key scopes.

---

## Task 1: schema + queries + default-page seed
**Files:** `api/internal/db/migrations/7_status_pages.{up,down}.sql`, `api/internal/db/queries/status_pages.sql`, regenerate.

- [ ] **Step 1** `7_status_pages.up.sql`:
```sql
CREATE TABLE status_pages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    domain     TEXT NOT NULL DEFAULT '' UNIQUE,
    title      TEXT NOT NULL DEFAULT 'Status',
    is_default INTEGER NOT NULL DEFAULT 0,
    published  INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE status_page_groups (
    status_page_id INTEGER NOT NULL REFERENCES status_pages(id) ON DELETE CASCADE,
    group_id       INTEGER NOT NULL,
    PRIMARY KEY (status_page_id, group_id)
);
INSERT INTO status_pages (domain, title, is_default, published) VALUES ('', 'Status', 1, 1);
```
`7_status_pages.down.sql`: `DROP TABLE status_page_groups; DROP TABLE status_pages;`
- [ ] **Step 2** `status_pages.sql`:
```sql
-- name: GetPageByDomain :one
SELECT * FROM status_pages WHERE domain = ?;

-- name: GetDefaultPage :one
SELECT * FROM status_pages WHERE is_default = 1 LIMIT 1;

-- name: ListStatusPages :many
SELECT * FROM status_pages ORDER BY is_default DESC, created_at ASC;

-- name: GetStatusPage :one
SELECT * FROM status_pages WHERE id = ?;

-- name: CreateStatusPage :one
INSERT INTO status_pages (domain, title, published) VALUES (?, ?, ?) RETURNING *;

-- name: UpdateStatusPage :exec
UPDATE status_pages SET domain = ?, title = ?, published = ? WHERE id = ?;

-- name: DeleteStatusPage :exec
DELETE FROM status_pages WHERE id = ? AND is_default = 0;

-- name: ListPageGroupIDs :many
SELECT group_id FROM status_page_groups WHERE status_page_id = ?;

-- name: ClearPageGroups :exec
DELETE FROM status_page_groups WHERE status_page_id = ?;

-- name: AddPageGroup :exec
INSERT INTO status_page_groups (status_page_id, group_id) VALUES (?, ?) ON CONFLICT DO NOTHING;
```
- [ ] **Step 3** regenerate + build. Report generated `StatusPage` fields + `CreateStatusPageParams`, `UpdateStatusPageParams`, `AddPageGroupParams`.
- [ ] **Step 4** smoke test: fresh DB has exactly one `is_default=1` page with `domain=''`.
- [ ] **Step 5: YOU COMMIT** — "feat(db): status_pages schema + default seed"

---

## Task 2: page resolver
**Files:** `api/internal/web/pages.go` (+ test).

- [ ] **Step 1: failing test** `pages_test.go`: with the seeded default page, `ResolvePage(ctx, q, "anything.com")` returns the default page (`IsDefault==1`) and `groupIDs == nil` (meaning "all groups"). After creating a published page `domain="acme.com"` with group 5, `ResolvePage(ctx, q, "acme.com")` returns that page with `groupIDs == [5]`. An unpublished non-default domain resolves to... return `(page, _, published=false)` — caller 404s. Design the signature so the caller knows published state, e.g.:
```go
type ResolvedPage struct {
    Page     generated.StatusPage
    GroupIDs []int64 // nil => all groups (default page)
}
func ResolvePage(ctx context.Context, q *generated.Queries, host string) (ResolvedPage, bool) // bool = serve (published)
```
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3** implement `ResolvePage`: strip port from host (`host, _, _ := net.SplitHostPort(host)` fallback to raw). `GetPageByDomain(host)`; on error → `GetDefaultPage()`. If the resolved page `IsDefault==1` → `GroupIDs=nil` (all), serve=true. Else `GroupIDs = ListPageGroupIDs(page.ID)`, serve = `page.Published==1`.
- [ ] **Step 4** run → PASS; build. **Step 5: YOU COMMIT** — "feat(web): host→page resolver"

---

## Task 3: public renderer uses the resolved page
**Files:** `api/internal/web/public.go`.

- [ ] **Step 1** Change `Get` to pass `r.Host`; `buildVM(ctx, rng, host)`. At the top of `buildVM`, `rp, serve := ResolvePage(ctx, p.q, host)`; if `!serve`, the `Get` handler should `http.NotFound` (return a sentinel from buildVM or check in Get before building — simplest: do `ResolvePage` in `Get`, 404 if `!serve`, else pass `rp` into `buildVM`).
- [ ] **Step 2** In `buildVM`, when `rp.GroupIDs != nil` build a `set := map[int64]bool` of allowed group IDs and **filter** `snap.Groups` to those in the set (default page: no filter). Use `rp.Page.Title` for `SiteName` (falls back to the theme `site_name`/"Status" if title empty). Collect the page's monitor IDs (monitors whose `GroupID` is in the allowed set; for default, all active monitors).
- [ ] **Step 3** **Scope incidents & maintenance**: only include an incident/maintenance window if its `affected_monitor_ids` intersects the page's monitor-ID set (for the default page, include all). Add a small `intersects(affectedJSON string, ids map[int64]bool) bool` helper (parse JSON array; true if any id in set; for default page where set is "all", pass a nil set meaning all-match).
- [ ] **Step 4** `go build ./... && go test ./internal/web/ -count=1` → green. Existing tests pass (default page → all groups, behaviour unchanged on the default host).
- [ ] **Step 5: YOU COMMIT** — "feat(public): render the host-resolved page (scoped groups + incidents)"

---

## Task 4: admin Pages CRUD + routes
**Files:** `api/internal/handlers/pages.go` (+ test), `server.go`, `middleware/apikey.go` (add `pages` scope).

- [ ] **Step 1: failing test**: `NewPages(q)`; Create (`{domain, title, published, group_ids:[]}`) → 201; List → includes default + the new one; Update sets groups; Delete (non-default) → 204; deleting the default page is refused (DeleteStatusPage has `AND is_default=0`, so it no-ops — return 400 if the target is default).
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3** implement `pages.go`: `List` (each page with its `group_ids` via `ListPageGroupIDs`), `Create` (CreateStatusPage then AddPageGroup per id), `Update` (UpdateStatusPage + ClearPageGroups + AddPageGroup loop), `Delete` (reject if `GetStatusPage().IsDefault==1` → 400, else DeleteStatusPage). View shape `{id, domain, title, is_default, published, group_ids:[]int64}`. Reuse `writeJSON`.
- [ ] **Step 4** run → PASS; build.
- [ ] **Step 5** `server.go`: in the `RequireSessionOrAPIKey` group add `/api/pages` routes (Get/Post/Put{id}/Delete{id}). Add to `requiredScope`: `case strings.HasPrefix(path, "/api/pages"): resource = "pages"`. (Add `pages:read|write` to the API-key UI scope list too — small edit to the api-keys page checklist.)
- [ ] **Step 6** `go build ./... && go test ./... -count=1` green. **Step 7: YOU COMMIT** — "feat(api): status-page CRUD"

---

## Task 5: admin Pages UI + nav
**Files:** `ui/src/app/admin/pages/page.tsx` (new), `ui/src/app/admin/layout.tsx` (nav), `ui/src/lib/api.ts`.

- [ ] **Step 1** `api.ts`: `listPages()`, `createPage(p)`, `updatePage(id, p)`, `deletePage(id)` (cookie). Type `StatusPage { id; domain; title; is_default; published; group_ids: number[] }`.
- [ ] **Step 2** `layout.tsx` NAV: add `{ href: '/admin/pages', label: 'Pages' }` (after Overview).
- [ ] **Step 3** `pages/page.tsx` (v3 look): list pages — title, domain (or "default" chip for the default page), published toggle, group count. "New page" modal: title, domain, published checkbox, and a checkbox list of monitor **groups** (from a `adminListGroups()` call — confirm it exists in api.ts). Default page row: domain shown as "— (default)", not deletable, but its groups/title editable. Non-default: editable + deletable (inline confirm). On save → create/update; refresh.
- [ ] **Step 4** build → succeeds; `ui/out/admin/pages/` present. **Step 5: YOU COMMIT** — "feat(admin): Pages management"

---

## Task 6: end-to-end verification
- [ ] **Step 1** `make build`/binary; fresh data dir; setup. Create groups + monitors (or seed). 
- [ ] **Step 2** Create a page `domain=acme.test`, published, with group A only.
- [ ] **Step 3** `curl -H 'Host: acme.test' localhost:PORT/` → shows only group A's monitors + page title; `curl localhost:PORT/` (default host) → shows all groups. `curl -H 'Host: unpublished.test'` for an unpublished page → 404.
- [ ] **Step 4** Confirm `/admin/pages` lists default + created page.
- [ ] **Step 5: YOU COMMIT** — "test: multi-page host routing verified"

---

## Self-Review
- Spec coverage (v1): status_pages + default ✅, host routing ✅, per-page group selection ✅, published ✅, admin Pages ✅, incident/maintenance scoping ✅. Deferred (noted): autocert TLS, per-page branding/theme/footer, per-page RSS.
- Consistency: default page = `domain=''`, `is_default=1`, all groups; non-default = own domain + selected groups, published-gated. `pages` scope added to apikey middleware + UI. `affected_monitor_ids` parsing reuses the JSON-array convention.
- Safety: default page cannot be deleted (query guard + handler 400). Unknown host → default (never 404 for the default).
