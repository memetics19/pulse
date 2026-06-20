# Pulse Plan 3b — Per-Page Branding (logo + theme)

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. `- [ ]` checkboxes.
> **PROJECT GIT RULE:** no git add/commit/push/mv. Stop at **YOU COMMIT**.

**Goal:** Each status page can have its own **logo** and **theme preset** (default-light/dark/terminal-dark), so different domains look like distinct branded sites. Empty values fall back to the global Theme config (today's behaviour). Title + group selection already exist (Plan 3).

**Scope:** per-page `logo_url` + `theme_preset` only. Per-page favicon / custom-CSS / footer stay global (the existing Theme page) for now.

**Builds on:** Plan 3 (status_pages, ResolvePage, public renderer, Pages admin).

---

## Task 1: columns + queries + handler view
**Files:** `api/internal/db/migrations/8_page_branding.{up,down}.sql`, `api/internal/db/queries/status_pages.sql` (update Create/Update), regenerate; `api/internal/handlers/pages.go` (view + create/update accept fields).

- [ ] **Step 1** `8_page_branding.up.sql`:
```sql
ALTER TABLE status_pages ADD COLUMN logo_url TEXT NOT NULL DEFAULT '';
ALTER TABLE status_pages ADD COLUMN theme_preset TEXT NOT NULL DEFAULT '';
```
`8_page_branding.down.sql`:
```sql
ALTER TABLE status_pages DROP COLUMN theme_preset;
ALTER TABLE status_pages DROP COLUMN logo_url;
```
- [ ] **Step 2** in `status_pages.sql`, update:
```sql
-- name: CreateStatusPage :one
INSERT INTO status_pages (domain, title, published, logo_url, theme_preset) VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateStatusPage :exec
UPDATE status_pages SET domain = ?, title = ?, published = ?, logo_url = ?, theme_preset = ? WHERE id = ?;
```
- [ ] **Step 3** regenerate; `go build ./...` will break `pages.go` (Create/Update call sites) — that's expected; fix in Step 4. Report new `CreateStatusPageParams`/`UpdateStatusPageParams` fields.
- [ ] **Step 4** `handlers/pages.go`: add `logo_url string` + `theme_preset string` to the request decode and the `pageView`; pass them to `CreateStatusPage`/`UpdateStatusPage`. (The default page may set these too.)
- [ ] **Step 5** `go build ./... && go test ./... -count=1` green (the existing pages_test still passes — new fields default to ""). **Step 6: YOU COMMIT** — "feat(db): per-page logo + theme_preset"

---

## Task 2: public renderer uses per-page branding
**Files:** `api/internal/web/public.go`, `api/internal/web/pages.go` (if needed).

- [ ] **Step 1** In `buildVM`, after resolving `rp` and parsing the global theme `cfg`: if `rp.Page.LogoURL != ""` → use it for `vm.LogoURL` (`template.URL(rp.Page.LogoURL)`), overriding the global logo. If `rp.Page.ThemePreset != ""` → set `vm.ThemePreset = rp.Page.ThemePreset` (overriding the global theme preset). (Title precedence already handled.)
- [ ] **Step 2** `go build ./... && go test ./internal/web/ -count=1` green.
- [ ] **Step 3: YOU COMMIT** — "feat(public): per-page logo + theme"

---

## Task 3: Pages admin UI — logo + theme
**Files:** `ui/src/lib/api.ts` (extend StatusPage + create/update payloads), `ui/src/app/admin/pages/page.tsx`.

- [ ] **Step 1** `api.ts`: add `logo_url: string; theme_preset: string` to `StatusPage`; add the same two fields to the `createPage`/`updatePage` payload params.
- [ ] **Step 2** `pages/page.tsx` modal: add a **Logo** uploader (file → data URL via `FileReader`, same pattern as the Theme page; with a preview + "Remove") bound to `logo_url`; and a **Theme** `<select>` (options: "" = "Use global theme", `default-light`, `default-dark`, `terminal-dark`) bound to `theme_preset`. Include both in the create/update calls. Pre-fill on edit.
- [ ] **Step 3** `cd ui && NEXT_PUBLIC_API_URL="" npm run build` → succeeds.
- [ ] **Step 4: YOU COMMIT** — "feat(admin): per-page logo + theme in Pages"

---

## Task 4: verify
- [ ] Build + run; create a page `domain=acme.test` with a logo data-URL + `theme_preset=terminal-dark`; `curl -H 'Host: acme.test' /` → the HTML has `data-theme="terminal-dark"` and the page's logo `<img>` (not the global brand); default host unchanged. **YOU COMMIT** — "test: per-page branding".

## Self-Review
- Empty per-page values fall back to global (no behaviour change for existing pages). `template.URL` keeps data-URL logos working (same as the global logo fix). Consistency: the three preset names match the Theme page + CSS `[data-theme=...]` blocks.
