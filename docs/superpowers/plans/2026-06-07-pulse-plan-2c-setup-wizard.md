# Pulse Plan 2c — First-Run Setup Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.
>
> **PROJECT GIT RULE:** Do NOT run git add/commit/push/mv. At each **YOU COMMIT** marker, stop for the human. Keep test-first discipline.

**Goal:** Replace the bare username/password setup with a gated, multi-step first-run wizard (Account → Branding → Database → Finish) that the whole app redirects to until configured.

**Architecture:** A `bootstrap` config file (`<data-dir>/pulse.json`, `{configured, sqlite_path}`) records setup state and the chosen SQLite path. The binary boots in **setup mode** when unconfigured: a swappable DB holder starts empty, the router serves only the wizard + `/api/setup*` + assets, and a gate redirects everything else to `/setup`. The wizard's Finish endpoint writes the bootstrap file, opens+migrates the chosen SQLite DB, creates the admin (name/email/username/password) and branding (logo/site name), then the app flips to configured. SQLite only for now — the DB step shows Postgres as "coming soon."

**Tech Stack:** Go (`net/http`, chi, `encoding/json`, `os`), sqlc/SQLite, golang-migrate, Argon2id (existing `auth` pkg), Next.js static export (wizard page), `go:embed`.

**Builds on:** Plan 2 (auth: `auth` pkg, sessions, `users`/`sessions` tables, `handlers.NewAuth`).

---

## File Structure

**Created:**
- `api/internal/bootstrap/bootstrap.go` (+ test) — read/write the bootstrap config file
- `api/internal/app/app.go` (+ test) — `App` holder with a swappable `*sql.DB`/`*generated.Queries` and `Configured()` state
- `api/internal/handlers/setup.go` (+ test) — `GET /api/setup/state`, `POST /api/setup`
- `api/internal/middleware/setupgate.go` (+ test) — redirect to `/setup` when unconfigured
- `api/internal/db/migrations/3_user_profile.{up,down}.sql` — add `name`, `email` to `users`
- `ui/src/app/setup/page.tsx` — multi-step wizard SPA page

**Modified:**
- `api/internal/db/queries/auth.sql` — `CreateUser` includes name/email
- `api/internal/server/server.go` — accept the `App` holder; mount `/api/setup*` + setup gate; routes resolve queries from the holder
- `api/cmd/pulse/main.go` — boot via bootstrap config; setup vs configured mode
- `ui/src/lib/api.ts` — `setupState()` / `runSetup()` client fns

---

## Task 1: `users` gains name + email

**Files:** `api/internal/db/migrations/3_user_profile.up.sql` / `.down.sql`; `api/internal/db/queries/auth.sql`; regenerate.

- [ ] **Step 1:** `3_user_profile.up.sql`:
```sql
ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';
```
- [ ] **Step 2:** `3_user_profile.down.sql`:
```sql
ALTER TABLE users DROP COLUMN email;
ALTER TABLE users DROP COLUMN name;
```
- [ ] **Step 3:** In `api/internal/db/queries/auth.sql`, replace `CreateUser` with:
```sql
-- name: CreateUser :one
INSERT INTO users (username, password_hash, name, email) VALUES (?, ?, ?, ?) RETURNING *;
```
- [ ] **Step 4:** Regenerate: `cd api/internal/db && sqlc generate` then `cd .. && go build ./...`. Confirm `CreateUserParams` now has `Username, PasswordHash, Name, Email`; `User` has `Name, Email`.
- [ ] **Step 5:** Migration test — extend `api/internal/db/db_test.go`:
```go
func TestUsersHaveProfileColumns(t *testing.T) {
	conn, err := Open(t.TempDir() + "/p.db")
	if err != nil { t.Fatal(err) }
	defer conn.Close()
	if _, err := conn.Exec(`INSERT INTO users (username, password_hash, name, email) VALUES ('a','h','A','a@b.c')`); err != nil {
		t.Fatalf("insert with name/email failed: %v", err)
	}
}
```
Run it → PASS.
- [ ] **Step 6: YOU COMMIT** — "feat(db): users.name + users.email"

> NOTE: The existing `handlers/auth.go` `Setup`/`Login` call `CreateUser` with the old 2-field params and will no longer compile. They are replaced by the new setup flow in Task 4; if you need an intermediate compile, pass `Name: "", Email: ""` until Task 4 rewrites them.

---

## Task 2: bootstrap config file

**Files:** `api/internal/bootstrap/bootstrap.go` + `bootstrap_test.go`

- [ ] **Step 1: failing test** `bootstrap_test.go`:
```go
package bootstrap

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if c, _ := Load(dir); c.Configured {
		t.Fatal("fresh dir must be unconfigured")
	}
	want := Config{Configured: true, SQLitePath: filepath.Join(dir, "pulse.db")}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Configured || got.SQLitePath != want.SQLitePath {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}
```
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: implement** `bootstrap.go`:
```go
package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the pre-database bootstrap state, stored as JSON on disk so the
// binary knows whether setup has run and which SQLite file to open.
type Config struct {
	Configured bool   `json:"configured"`
	SQLitePath string `json:"sqlite_path"`
}

func file(dataDir string) string { return filepath.Join(dataDir, "pulse.json") }

// Load reads the bootstrap config; a missing file yields a zero Config (unconfigured).
func Load(dataDir string) (Config, error) {
	b, err := os.ReadFile(file(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes the bootstrap config, creating the data dir if needed.
func Save(dataDir string, c Config) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file(dataDir), b, 0o600)
}
```
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: YOU COMMIT** — "feat(bootstrap): setup-state config file"

---

## Task 3: App holder (swappable DB + configured state)

**Files:** `api/internal/app/app.go` + `app_test.go`

- [ ] **Step 1: failing test** `app_test.go`:
```go
package app

import (
	"testing"

	"github.com/memetics19/pulse/api/testutil"
)

func TestAppConfiguredFlips(t *testing.T) {
	a := New()
	if a.Configured() {
		t.Fatal("new app must be unconfigured")
	}
	if a.Queries() != nil {
		t.Fatal("no queries before configuration")
	}
	db := testutil.NewTestDB(t)
	a.SetDB(db)
	if !a.Configured() || a.Queries() == nil {
		t.Fatal("SetDB must make the app configured with queries")
	}
}
```
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: implement** `app.go`:
```go
package app

import (
	"database/sql"
	"sync"

	"github.com/memetics19/pulse/api/internal/generated"
)

// App holds the (initially absent) database so the server can run in setup
// mode and flip to configured once setup completes.
type App struct {
	mu sync.RWMutex
	db *sql.DB
	q  *generated.Queries
}

func New() *App { return &App{} }

// SetDB installs an open, migrated database and marks the app configured.
func (a *App) SetDB(db *sql.DB) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.db = db
	a.q = generated.New(db)
}

func (a *App) Configured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.db != nil
}

// Queries returns the query handle, or nil when unconfigured.
func (a *App) Queries() *generated.Queries {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.q
}
```
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: YOU COMMIT** — "feat(app): swappable DB holder"

---

## Task 4: Setup API (state + complete)

**Files:** `api/internal/handlers/setup.go` + `setup_test.go`. Modifies `handlers/auth.go` (CreateUser call signature).

- [ ] **Step 1: failing test** `setup_test.go`:
```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
)

func TestSetupStateAndComplete(t *testing.T) {
	dataDir := t.TempDir()
	a := app.New()
	h := NewSetup(a, dataDir, false)

	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
	var st struct{ Configured bool `json:"configured"` }
	json.NewDecoder(rec.Body).Decode(&st)
	if st.Configured {
		t.Fatal("fresh app: configured should be false")
	}

	body, _ := json.Marshal(map[string]any{
		"name": "Shreeda", "email": "s@b.c", "username": "admin", "password": "supersecret",
		"site_name": "Acme", "logo_url": "", "sqlite_path": dataDir + "/pulse.db",
	})
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup complete = %d, want 201", rec.Code)
	}
	if !a.Configured() {
		t.Fatal("app should be configured after setup")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should log the admin in (session cookie)")
	}

	// Second attempt is rejected
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("second setup = %d, want 403", rec.Code)
	}
}
```
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: implement** `setup.go`:
```go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

type Setup struct {
	app     *app.App
	dataDir string
	secure  bool
}

func NewSetup(a *app.App, dataDir string, secure bool) *Setup {
	return &Setup{app: a, dataDir: dataDir, secure: secure}
}

func (s *Setup) State(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":           s.app.Configured(),
		"default_sqlite_path":  s.dataDir + "/pulse.db",
	})
}

type setupReq struct {
	Name, Email, Username, Password string
	SiteName                        string `json:"site_name"`
	LogoURL                         string `json:"logo_url"`
	SQLitePath                      string `json:"sqlite_path"`
}

func (s *Setup) Complete(w http.ResponseWriter, r *http.Request) {
	if s.app.Configured() {
		http.Error(w, "already configured", http.StatusForbidden)
		return
	}
	var req setupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Username == "" || len(req.Password) < 8 {
		http.Error(w, "username and password (min 8) required", http.StatusBadRequest)
		return
	}
	path := req.SQLitePath
	if path == "" {
		path = s.dataDir + "/pulse.db"
	}

	conn, err := db.Open(path) // opens + migrates
	if err != nil {
		http.Error(w, "could not open database: "+err.Error(), http.StatusBadRequest)
		return
	}
	q := generated.New(conn)

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}
	u, err := q.CreateUser(r.Context(), generated.CreateUserParams{
		Username: req.Username, PasswordHash: hash, Name: req.Name, Email: req.Email,
	})
	if err != nil {
		http.Error(w, "could not create admin", http.StatusInternalServerError)
		return
	}

	// Branding -> theme config (best-effort).
	cfg, _ := json.Marshal(map[string]string{"site_name": req.SiteName, "logo_url": req.LogoURL})
	_ = q.UpdateTheme(r.Context(), generated.UpdateThemeParams{
		Preset: "default-light", CustomCss: "", ConfigJson: string(cfg),
	})

	if err := bootstrap.Save(s.dataDir, bootstrap.Config{Configured: true, SQLitePath: path}); err != nil {
		http.Error(w, "could not persist config", http.StatusInternalServerError)
		return
	}
	s.app.SetDB(conn)

	// Log the new admin in.
	token, err := auth.NewSessionToken()
	if err == nil {
		_ = q.CreateSession(r.Context(), generated.CreateSessionParams{
			Token: token, UserID: u.ID, ExpiresAt: time.Now().Add(auth.SessionDuration),
		})
		auth.SetSessionCookie(w, token, s.secure)
	}
	w.WriteHeader(http.StatusCreated)
}
```
> Before coding: confirm the theme update query/params name (`UpdateTheme` / `UpdateThemeParams`) by reading `api/internal/generated/theme.sql.go`; adjust the call to match. Confirm `db.Open` runs migrations (it does). Update `handlers/auth.go`'s existing `CreateUser` call to pass `Name`/`Email` (empty is fine there) so the package compiles — though the legacy `/api/auth/setup` is superseded by this wizard (server wiring in Task 6 drops it).

- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: YOU COMMIT** — "feat(setup): wizard state + complete endpoints"

---

## Task 5: setup-gate middleware

**Files:** `api/internal/middleware/setupgate.go` + test

- [ ] **Step 1: failing test** `setupgate_test.go`:
```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
)

func TestSetupGateRedirectsWhenUnconfigured(t *testing.T) {
	a := app.New()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := SetupGate(a)(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/setup/" {
		t.Fatalf("unconfigured / should redirect to /setup/, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
```
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3: implement** `setupgate.go`:
```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/memetics19/pulse/api/internal/app"
)

// SetupGate redirects all traffic to /setup/ until the app is configured.
// Paths needed BY the wizard (the wizard page, its assets, and setup APIs)
// pass through so the wizard can render and submit.
func SetupGate(a *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.Configured() || allowedDuringSetup(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, "/setup/", http.StatusFound)
		})
	}
}

func allowedDuringSetup(p string) bool {
	return p == "/setup" || strings.HasPrefix(p, "/setup/") ||
		strings.HasPrefix(p, "/_next/") || strings.HasPrefix(p, "/static/") ||
		strings.HasPrefix(p, "/api/setup") || p == "/favicon.ico" || p == "/healthz"
}
```
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: YOU COMMIT** — "feat(setup): gate middleware"

---

## Task 6: Wire the server + entrypoint to the App holder + setup mode

**Files:** `api/internal/server/server.go`, `api/cmd/pulse/main.go`, `api/cmd/pulse/main_test.go`

- [ ] **Step 1:** Change `server.New` to take the holder + data dir: `func New(a *app.App, dataDir string) http.Handler`. Inside:
  - Apply `middleware.SetupGate(a)` as global middleware (after Logger/Recoverer/CORS).
  - Mount setup routes (always): `setupH := handlers.NewSetup(a, dataDir, false)`; `r.Get("/api/setup/state", setupH.State)`; `r.Post("/api/setup", setupH.Complete)`.
  - For all routes that need the DB (status, monitors, auth login/logout/status, admin group, public page), resolve the queries from `a.Queries()` **per request** (the handlers currently capture `q` at wiring time). Simplest: wrap DB-dependent handlers so they fetch `a.Queries()` at call time and 503 if nil. Provide a small helper:
    ```go
    func needDB(a *app.App, fn func(*generated.Queries) http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            q := a.Queries()
            if q == nil { http.Error(w, "not configured", http.StatusServiceUnavailable); return }
            fn(q)(w, r)
        }
    }
    ```
    and register e.g. `r.Get("/api/status", needDB(a, func(q *generated.Queries) http.HandlerFunc { return handlers.NewStatus(q).Get }))`. Apply the same wrapper to every DB-backed route (monitors, incidents, groups, agents, theme, notifications, checks, incident-updates, auth login/logout/status, public `/`). The setup gate already redirects browsers pre-config, so this is a belt-and-suspenders 503 for API clients.
  - Keep `/admin/*`, `/_next/*`, `/static/*`, `/favicon.ico` served as today.
- [ ] **Step 2:** `cmd/pulse/main.go`:
  - Determine `dataDir` (env `PULSE_DATA_DIR`, else dir of `SQLITE_PATH`, else `/data`).
  - `boot, _ := bootstrap.Load(dataDir)`.
  - `a := app.New()`. If `boot.Configured`, open `db.Open(boot.SQLitePath)` and `a.SetDB(conn)`; the worker starts only once configured. If not configured, start in setup mode (no DB, no worker).
  - `srv := server.New(a, dataDir)`. Serve as before.
  - The worker (`worker.Run`) must only run with a DB — start it after `a.SetDB`, or have it poll until configured. Simplest for v1: if configured at boot, start the worker; otherwise start it after setup completes. To keep main simple, start a goroutine that waits for `a.Configured()` to become true (poll every 2s) then runs `worker.Run(ctx, a.DB(), cfg)`. (Add an `App.DB()` accessor.)
  - The legacy `/api/auth/setup` route is removed (the wizard replaces it); `/api/auth/login|logout|status` stay (resolved via `needDB`).
- [ ] **Step 3:** Update `main_test.go` (`server.New(db, ...)` → build an `app.App` with the test DB and call `server.New(a, t.TempDir())`); the healthz test still expects 200 (healthz is allowed during setup).
- [ ] **Step 4:** `go build ./... && go test ./... -count=1` → all green. Fix any handler that still captured `q` at wiring time.
- [ ] **Step 5: YOU COMMIT** — "feat(setup): server boots in setup mode, gated, App-backed"

---

## Task 7: Wizard UI (multi-step) + client

**Files:** `ui/src/app/setup/page.tsx`, `ui/src/lib/api.ts`

- [ ] **Step 1:** Add to `ui/src/lib/api.ts`:
```ts
export type SetupState = { configured: boolean; default_sqlite_path: string }
export async function setupState(): Promise<SetupState> {
  const res = await fetch(`${BASE}/api/setup/state`, { cache: 'no-store' })
  return res.json()
}
export async function runSetup(payload: {
  name: string; email: string; username: string; password: string
  site_name: string; logo_url: string; sqlite_path: string
}): Promise<void> {
  const res = await fetch(`${BASE}/api/setup`, {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
  })
  if (!res.ok) throw new Error(await res.text())
}
```
- [ ] **Step 2:** Create `ui/src/app/setup/page.tsx` — a `'use client'` 3-step wizard reusing the framed-card style from the admin auth screen:
  - **Step 1 Account:** name, email, username, password, confirm password (min 8, must match).
  - **Step 2 Branding:** site name; logo upload (FileReader → data URL, same as the Theme page) with a preview.
  - **Step 3 Database:** radio "SQLite (recommended)" selected; an editable SQLite path defaulting to `state.default_sqlite_path`; a disabled "PostgreSQL — coming soon" radio. A "Finish setup" button calls `runSetup(...)`; on success `window.location.href = '/admin/'`.
  - A stepper header (1 Account · 2 Branding · 3 Database) and Back/Next buttons. On mount, call `setupState()`; if already `configured`, redirect to `/admin/`.
  - Use the `Logo` mark (the Pul⚡se bolt) at the top. Match the modern/formal card aesthetic (white card, hairline border, soft shadow, indigo primary button) — no gradients/glow.
  > Full component code: model it on the existing `AuthScreen` card in `ui/src/app/admin/layout.tsx` (reuse the same card container styles and `.form-input`/`.btn primary` classes); render one step's fields at a time based on a `step` state (1|2|3).
- [ ] **Step 3:** `cd ui && NEXT_PUBLIC_API_URL="" npm run build` → confirm `ui/out/setup/` is exported. Fix any TS errors.
- [ ] **Step 4: YOU COMMIT** — "feat(admin): multi-step setup wizard UI"

---

## Task 8: End-to-end verification

- [ ] **Step 1:** `make build`; run on a SPARE port with a FRESH data dir:
```bash
PULSE_DATA_DIR=/tmp/pulse-setup SQLITE_PATH=/tmp/pulse-setup/pulse.db API_PORT=8090 ./bin/pulse &
```
(ensure `/tmp/pulse-setup` is empty first: `rm -rf /tmp/pulse-setup`)
- [ ] **Step 2:** Verify gating + flow with curl:
```bash
echo "setup state:" && curl -s localhost:8090/api/setup/state
echo "/ redirects to /setup (expect 302 -> /setup/):" && curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' localhost:8090/
echo "/admin redirects too:" && curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' localhost:8090/admin/
echo "complete setup:" && curl -s -i -c /tmp/sj -X POST localhost:8090/api/setup -H 'Content-Type: application/json' \
  -d '{"name":"Shreeda","email":"s@b.c","username":"admin","password":"supersecret","site_name":"Acme","logo_url":"","sqlite_path":"/tmp/pulse-setup/pulse.db"}' | grep -i 'HTTP/\|set-cookie'
echo "state after (configured true):" && curl -s localhost:8090/api/setup/state
echo "/ now serves public page (200):" && curl -s -o /dev/null -w '%{http_code}\n' localhost:8090/
echo "bootstrap file:" && cat /tmp/pulse-setup/pulse.json
```
Expected: state `configured:false` → `/` and `/admin/` 302 → `/setup/`; complete → 201 + Set-Cookie; state `configured:true`; `/` → 200; `pulse.json` shows `configured:true`.
- [ ] **Step 3:** Restart the binary against the same data dir → it boots **configured** (reads `pulse.json`), `/` serves the public page directly, `/setup/` redirects to `/admin/`.
- [ ] **Step 4:** Headless screenshot of `/setup/` on a fresh data dir to confirm the wizard renders.
- [ ] **Step 5: YOU COMMIT** — "test: end-to-end setup wizard flow"

---

## Self-Review notes

- **Spec coverage:** gate-until-setup ✅, name/email/password ✅, logo/site-name ✅, multi-step UI ✅, SQLite DB step ✅ (Postgres explicitly "coming soon" — its own future plan).
- **Supersedes:** the Plan 2 `/api/auth/setup` endpoint + the admin layout's setup branch (the wizard at `/setup` now owns first-run; the admin layout keeps only the login branch). Update `layout.tsx` to drop the `needs_setup` branch (or leave it — it'll never trigger once the gate redirects, but removing it is cleaner). 
- **Worker:** starts only once configured (poll-for-configured goroutine).
- **Risk:** every DB-backed route must resolve queries from `App` at request time, not capture a nil `q` at wiring. The `needDB` wrapper enforces this — audit that no handler is wired with a captured `q`.
