# Pulse Plan 2 — Authentication (login) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.
>
> **PROJECT GIT RULE (overrides skill defaults):** Do **NOT** run `git add`, `git commit`, `git push`, or `git mv`. Wherever a step says **YOU COMMIT**, stop and let the human commit. Keep test-first discipline; never invoke git.

**Goal:** Replace the static-token admin gate with real authentication — a first-run setup screen, username/password login with Argon2id hashing, and HttpOnly cookie sessions (Uptime-Kuma-style, single admin).

**Architecture:** New `users` + `sessions` tables (golang-migrate migration + sqlc queries). A small `auth` package wraps Argon2id hashing and secure session-token generation. Auth HTTP handlers (`/api/auth/{status,setup,login,logout}`) issue/clear an HttpOnly cookie. A `RequireSession` middleware replaces `RequireToken` on the admin route group. The admin SPA gets a setup/login/logout flow and switches every admin API call from a `Bearer` header to `credentials: 'include'` (cookie). A `pulse-cli reset-password` command covers lockout recovery.

**Tech Stack:** Go (`net/http`, chi, `crypto/rand`), `github.com/alexedwards/argon2id`, sqlc, golang-migrate, modernc SQLite, Next.js (admin SPA), cobra (CLI).

**Scope / deferred:** This plan delivers setup + login + sessions + logout + CLI reset. **TOTP 2FA and API keys are deferred to a follow-up plan (2b)** — the `users` table includes nullable `totp_secret`/`totp_enabled` columns now so TOTP later needs no migration.

---

## File Structure

**Created:**
- `api/internal/db/migrations/2_auth.up.sql` / `2_auth.down.sql` — `users`, `sessions` tables
- `api/internal/db/queries/auth.sql` — sqlc queries (regenerates into `api/internal/generated/auth.sql.go`)
- `api/internal/auth/password.go` — Argon2id hash/verify
- `api/internal/auth/session.go` — session token generation + cookie constants/helpers
- `api/internal/auth/password_test.go`, `api/internal/auth/session_test.go`
- `api/internal/handlers/auth.go` — status/setup/login/logout handlers
- `api/internal/handlers/auth_test.go`
- `api/internal/middleware/session.go` — `RequireSession`
- `api/internal/middleware/session_test.go`
- `api/internal/account/account.go` + test — password-reset logic
- (subcommand wiring in `api/cmd/pulse/main.go` — `pulse reset-password`)

**Modified:**
- `api/internal/server/server.go` — drop `adminToken`; add `/api/auth/*` (public) + `RequireSession(q)` on the admin group
- `api/cmd/pulse/main.go` — `server.New(conn)` (signature change)
- `api/cmd/pulse/main_test.go` — update `server.New` call
- `ui/src/app/admin/layout.tsx` — setup/login/logout flow (cookie-based)
- `ui/src/lib/api.ts` — drop `token` params, add `credentials: 'include'`
- `ui/src/app/admin/*/page.tsx` — remove `getToken()` and token args from `admin*` calls
- `api/go.mod` — add `github.com/alexedwards/argon2id`

---

## Task 1: Auth schema migration + sqlc queries

**Files:**
- Create: `api/internal/db/migrations/2_auth.up.sql`, `api/internal/db/migrations/2_auth.down.sql`
- Create: `api/internal/db/queries/auth.sql`
- Regenerate: `api/internal/generated/*` via `make sqlc`

- [ ] **Step 1: Write the up migration** `api/internal/db/migrations/2_auth.up.sql`:
```sql
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret   TEXT,
    totp_enabled  INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);
```

- [ ] **Step 2: Write the down migration** `api/internal/db/migrations/2_auth.down.sql`:
```sql
DROP TABLE sessions;
DROP TABLE users;
```

- [ ] **Step 3: Write the sqlc queries** `api/internal/db/queries/auth.sql`:
```sql
-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateUser :one
INSERT INTO users (username, password_hash) VALUES (?, ?) RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ? WHERE username = ?;

-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?);

-- name: GetSession :one
SELECT s.token, s.user_id, s.expires_at, u.username
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP;
```
> Note: `GetSession` does NOT filter by expiry in SQL (CURRENT_TIMESTAMP/stored-format mismatch is error-prone). Expiry is checked in Go (Task 4/5).

- [ ] **Step 4: Regenerate sqlc + migrate-build check**

Run: `cd /Users/shreedabhat/Documents/statuspage/pulse/api && make -C .. sqlc 2>/dev/null || (cd internal/db && sqlc generate)` then `go build ./...`
Expected: `api/internal/generated/auth.sql.go` created with `CountUsers`, `CreateUser`, `GetUserByUsername`, `GetUserByID`, `UpdateUserPassword`, `CreateSession`, `GetSession`, `DeleteSession`, `DeleteExpiredSessions`; build succeeds.
> Verify generated method/struct names. `CreateUser`/`GetUserByUsername` return a `generated.User` with fields `ID, Username, PasswordHash, TotpSecret (*string), TotpEnabled int64, CreatedAt time.Time`. `GetSession` returns a row struct (e.g. `generated.GetSessionRow`) with `Token, UserID, ExpiresAt, Username`. Use the actual generated names in later tasks.

- [ ] **Step 5: Migration smoke test** — confirm a fresh DB migrates with the new tables.

`api/internal/db/db_test.go` already tests `Open`; add a check there (or a new test) that the `users` table exists:
```go
func TestMigrationsCreateUsersTable(t *testing.T) {
	conn, err := Open(t.TempDir() + "/m.db")
	if err != nil { t.Fatal(err) }
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("users table missing: %v", err)
	}
}
```
Run: `cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/db/ -run TestMigrationsCreateUsersTable -v`
Expected: PASS.

- [ ] **Step 6: YOU COMMIT** — "feat(db): users + sessions schema and queries"

---

## Task 2: Argon2id password hashing

**Files:**
- Create: `api/internal/auth/password.go`, `api/internal/auth/password_test.go`
- Modify: `api/go.mod` (add `github.com/alexedwards/argon2id`)

- [ ] **Step 1: Add the dependency**

Run: `cd /Users/shreedabhat/Documents/statuspage/pulse/api && go get github.com/alexedwards/argon2id@latest && go mod tidy`

- [ ] **Step 2: Write the failing test** `api/internal/auth/password_test.go`:
```go
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" || hash == "correct horse battery staple" {
		t.Fatal("hash must be non-empty and not the plaintext")
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}

	bad, err := VerifyPassword("wrong", hash)
	if err != nil {
		t.Fatalf("verify wrong password errored: %v", err)
	}
	if bad {
		t.Fatal("wrong password must not verify")
	}
}
```

- [ ] **Step 3: Run test, confirm FAILS** (undefined: HashPassword):
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/auth/ -run TestHashAndVerifyPassword -v`

- [ ] **Step 4: Implement** `api/internal/auth/password.go`:
```go
package auth

import "github.com/alexedwards/argon2id"

// params: OWASP-recommended, lightweight enough for a single rare admin login.
var argonParams = &argon2id.Params{
	Memory:      19 * 1024, // 19 MiB
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// HashPassword returns an encoded Argon2id hash (includes salt + params).
func HashPassword(plain string) (string, error) {
	return argon2id.CreateHash(plain, argonParams)
}

// VerifyPassword reports whether plain matches the encoded hash.
func VerifyPassword(plain, encodedHash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plain, encodedHash)
}
```

- [ ] **Step 5: Run test, confirm PASS:**
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/auth/ -run TestHashAndVerifyPassword -v`

- [ ] **Step 6: YOU COMMIT** — "feat(auth): Argon2id password hashing"

---

## Task 3: Session tokens + cookie helper

**Files:**
- Create: `api/internal/auth/session.go`, `api/internal/auth/session_test.go`

- [ ] **Step 1: Write the failing test** `api/internal/auth/session_test.go`:
```go
package auth

import (
	"net/http/httptest"
	"testing"
)

func TestNewSessionTokenIsRandom(t *testing.T) {
	a, err := NewSessionToken()
	if err != nil { t.Fatal(err) }
	b, err := NewSessionToken()
	if err != nil { t.Fatal(err) }
	if a == "" || a == b {
		t.Fatalf("tokens must be non-empty and unique: %q %q", a, b)
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, "tok123", false)
	c := rec.Result().Cookies()
	if len(c) != 1 || c[0].Name != SessionCookieName || c[0].Value != "tok123" || !c[0].HttpOnly {
		t.Fatalf("unexpected cookie: %+v", c)
	}
	rec2 := httptest.NewRecorder()
	ClearSessionCookie(rec2)
	c2 := rec2.Result().Cookies()
	if len(c2) != 1 || c2[0].MaxAge >= 0 {
		t.Fatalf("expected an expiring cookie, got: %+v", c2)
	}
}
```

- [ ] **Step 2: Run test, confirm FAILS:**
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/auth/ -run 'Session' -v`

- [ ] **Step 3: Implement** `api/internal/auth/session.go`:
```go
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"
)

const (
	SessionCookieName = "pulse_session"
	SessionDuration   = 30 * 24 * time.Hour
)

// NewSessionToken returns a 256-bit URL-safe random token.
func NewSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetSessionCookie writes the session cookie. secure=true sets the Secure flag
// (use when serving over HTTPS).
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}
```

- [ ] **Step 4: Run test, confirm PASS:**
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/auth/ -run 'Session' -v`

- [ ] **Step 5: YOU COMMIT** — "feat(auth): session tokens and cookie helpers"

---

## Task 4: Auth HTTP handlers (status / setup / login / logout)

**Files:**
- Create: `api/internal/handlers/auth.go`, `api/internal/handlers/auth_test.go`

- [ ] **Step 1: Write the failing test** `api/internal/handlers/auth_test.go`:
```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestAuthSetupThenLoginThenStatus(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := NewAuth(q, false)

	// Fresh DB → needs setup
	rec := httptest.NewRecorder()
	h.Status(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	var st struct{ NeedsSetup, Authenticated bool }
	json.NewDecoder(rec.Body).Decode(&st)
	if !st.NeedsSetup || st.Authenticated {
		t.Fatalf("fresh DB: want needs_setup=true authenticated=false, got %+v", st)
	}

	// Setup creates the admin and logs in (sets cookie)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})
	rec = httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d, want 201", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should set a session cookie")
	}

	// Second setup is forbidden
	rec = httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("second setup = %d, want 403", rec.Code)
	}

	// Login with correct creds → 200 + cookie
	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login = %d, cookies=%d", rec.Code, len(rec.Result().Cookies()))
	}

	// Login with wrong password → 401
	bad, _ := json.Marshal(map[string]string{"username": "admin", "password": "nope"})
	rec = httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(bad)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run test, confirm FAILS** (undefined: NewAuth):
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/handlers/ -run TestAuthSetupThenLoginThenStatus -v`

- [ ] **Step 3: Implement** `api/internal/handlers/auth.go`:
```go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

type Auth struct {
	q      *generated.Queries
	secure bool // set Secure flag on cookies (HTTPS)
}

func NewAuth(q *generated.Queries, secure bool) *Auth { return &Auth{q: q, secure: secure} }

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *Auth) Status(w http.ResponseWriter, r *http.Request) {
	n, _ := a.q.CountUsers(r.Context())
	authenticated := false
	username := ""
	if c, err := r.Cookie(auth.SessionCookieName); err == nil {
		if sess, err := a.q.GetSession(r.Context(), c.Value); err == nil && sess.ExpiresAt.After(time.Now()) {
			authenticated = true
			username = sess.Username
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup":   n == 0,
		"authenticated": authenticated,
		"username":      username,
	})
}

func (a *Auth) Setup(w http.ResponseWriter, r *http.Request) {
	n, _ := a.q.CountUsers(r.Context())
	if n > 0 {
		http.Error(w, "already set up", http.StatusForbidden)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.Username == "" || len(c.Password) < 8 {
		http.Error(w, "username and password (min 8 chars) required", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}
	u, err := a.q.CreateUser(r.Context(), generated.CreateUserParams{Username: c.Username, PasswordHash: hash})
	if err != nil {
		http.Error(w, "could not create user", http.StatusInternalServerError)
		return
	}
	a.startSession(w, r, u.ID)
	w.WriteHeader(http.StatusCreated)
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	u, err := a.q.GetUserByUsername(r.Context(), c.Username)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	ok, err := auth.VerifyPassword(c.Password, u.PasswordHash)
	if err != nil || !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	a.startSession(w, r, u.ID)
	w.WriteHeader(http.StatusOK)
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = a.q.DeleteSession(r.Context(), c.Value)
	}
	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusOK)
}

func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, userID int64) {
	token, err := auth.NewSessionToken()
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	_ = a.q.CreateSession(r.Context(), generated.CreateSessionParams{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(auth.SessionDuration),
	})
	auth.SetSessionCookie(w, token, a.secure)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```
> Before coding: confirm there isn't already a `writeJSON` helper in `handlers` (grep). If one exists, reuse it and delete this duplicate. Confirm generated param struct names (`CreateUserParams`, `CreateSessionParams`) and the `GetSession` row field names (`ExpiresAt`, `Username`) match Task 1's generated output; adjust if different.

- [ ] **Step 4: Run test, confirm PASS:**
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/handlers/ -run TestAuthSetupThenLoginThenStatus -v`

- [ ] **Step 5: YOU COMMIT** — "feat(auth): setup/login/logout/status handlers"

---

## Task 5: RequireSession middleware + wire into the server

**Files:**
- Create: `api/internal/middleware/session.go`, `api/internal/middleware/session_test.go`
- Modify: `api/internal/server/server.go`, `api/cmd/pulse/main.go`, `api/cmd/pulse/main_test.go`

- [ ] **Step 1: Write the failing middleware test** `api/internal/middleware/session_test.go`:
```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestRequireSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RequireSession(q)(ok)

	// No cookie → 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/monitors", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie = %d, want 401", rec.Code)
	}

	// Valid session → 200
	hash, _ := auth.HashPassword("pw-at-least-8")
	u, _ := q.CreateUser(t.Context(), generated.CreateUserParams{Username: "a", PasswordHash: hash})
	tok, _ := auth.NewSessionToken()
	_ = q.CreateSession(t.Context(), generated.CreateSessionParams{Token: tok, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})

	req := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: tok})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid session = %d, want 200", rec.Code)
	}
}
```
> If `t.Context()` is unavailable in this Go version, use `context.Background()`.

- [ ] **Step 2: Run test, confirm FAILS** (undefined: RequireSession):
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/middleware/ -run TestRequireSession -v`

- [ ] **Step 3: Implement** `api/internal/middleware/session.go`:
```go
package middleware

import (
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

// RequireSession allows the request only if it carries a valid, unexpired
// session cookie.
func RequireSession(q *generated.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(auth.SessionCookieName)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			sess, err := q.GetSession(r.Context(), c.Value)
			if err != nil || !sess.ExpiresAt.After(time.Now()) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Run test, confirm PASS.**

- [ ] **Step 5: Wire the server.** In `api/internal/server/server.go`:
  - Change signature to `func New(db *sql.DB) http.Handler` (drop `adminToken`).
  - Add public auth routes near the other public routes:
    ```go
    authH := handlers.NewAuth(q, false) // secure=false for HTTP; Plan 3 sets true under TLS
    r.Post("/api/auth/setup", authH.Setup)
    r.Post("/api/auth/login", authH.Login)
    r.Post("/api/auth/logout", authH.Logout)
    r.Get("/api/auth/status", authH.Status)
    ```
  - Replace `r.Use(middleware.RequireToken(adminToken))` in the admin group with `r.Use(middleware.RequireSession(q))`.
  - Delete the now-unused `RequireToken` from `api/internal/middleware/auth.go` (and its test `auth_test.go`) — grep to confirm nothing else uses it.

- [ ] **Step 6: Update the entrypoint + its test.**
  - `api/cmd/pulse/main.go`: `srv := server.New(conn)` (drop `cfg.AdminToken`). `cfg.AdminToken` may now be unused — leave the config field (harmless) or remove if nothing references it.
  - `api/cmd/pulse/main_test.go`: change `server.New(db, "test-token")` → `server.New(db)`.

- [ ] **Step 7: Verify full build + tests:**
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go build ./... && go test ./... -count=1`
Expected: build + all tests PASS.

- [ ] **Step 8: YOU COMMIT** — "feat(auth): session middleware; replace static token gate"

---

## Task 6: Admin SPA — setup / login / logout (cookie-based)

**Files:**
- Modify: `ui/src/lib/api.ts` (drop `token` params, add `credentials: 'include'`)
- Modify: `ui/src/app/admin/layout.tsx` (auth flow)
- Modify: `ui/src/app/admin/{monitors,incidents,agents,notifications,theme}/page.tsx` (remove token args)

- [ ] **Step 1: Switch the API client to cookies.** In `ui/src/lib/api.ts`:
  - Replace `adminHeaders(token)` with a constant `const ADMIN_HEADERS = { 'Content-Type': 'application/json' }`.
  - Remove the `token: string` first parameter from EVERY `admin*` function.
  - Add `credentials: 'include'` to EVERY `admin*` fetch call (so the session cookie is sent).
  - Add auth client functions:
    ```ts
    export type AuthStatus = { needs_setup: boolean; authenticated: boolean; username: string }
    export async function authStatus(): Promise<AuthStatus> {
      const res = await fetch(`${BASE}/api/auth/status`, { credentials: 'include', cache: 'no-store' })
      return res.json()
    }
    export async function authSetup(username: string, password: string): Promise<void> {
      const res = await fetch(`${BASE}/api/auth/setup`, { method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) })
      if (!res.ok) throw new Error(await res.text())
    }
    export async function authLogin(username: string, password: string): Promise<void> {
      const res = await fetch(`${BASE}/api/auth/login`, { method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) })
      if (!res.ok) throw new Error('Invalid username or password')
    }
    export async function authLogout(): Promise<void> {
      await fetch(`${BASE}/api/auth/logout`, { method: 'POST', credentials: 'include' })
    }
    ```

- [ ] **Step 2: Update every admin page call site.** In each of `ui/src/app/admin/{monitors,incidents,agents,notifications,theme}/page.tsx`:
  - Remove the local `getToken()` helper and any `localStorage.getItem('pulse-admin-token')`.
  - Remove the `token` argument from every `admin*(...)` call (e.g. `adminListMonitors(getToken())` → `adminListMonitors()`).
  - These are mechanical edits; the TypeScript compiler (`npm run build`) will flag any you miss.

- [ ] **Step 3: Rewrite the admin layout auth gate.** Replace `ui/src/app/admin/layout.tsx` with:
```tsx
'use client'
import { useState, useEffect, useCallback } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { authStatus, authSetup, authLogin, authLogout, type AuthStatus } from '@/lib/api'

const NAV = [
  { href: '/admin/monitors',      label: 'Monitors' },
  { href: '/admin/incidents',     label: 'Incidents' },
  { href: '/admin/theme',         label: 'Theme' },
  { href: '/admin/notifications', label: 'Notifications' },
  { href: '/admin/agents',        label: 'Agents' },
]

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  const refresh = useCallback(() => {
    authStatus().then(setStatus).catch(() => setStatus({ needs_setup: false, authenticated: false, username: '' }))
  }, [])
  useEffect(() => { refresh() }, [refresh])

  async function submitSetup(e: React.FormEvent) {
    e.preventDefault(); setError('')
    try { await authSetup(username, password); setPassword(''); refresh() }
    catch (err) { setError(err instanceof Error ? err.message : 'Setup failed') }
  }
  async function submitLogin(e: React.FormEvent) {
    e.preventDefault(); setError('')
    try { await authLogin(username, password); setPassword(''); refresh() }
    catch (err) { setError(err instanceof Error ? err.message : 'Login failed') }
  }
  async function signOut() { await authLogout(); refresh() }

  if (status === null) return null

  if (status.needs_setup) {
    return (
      <div className="wrap token-form">
        <h1>Welcome to Pulse</h1>
        <p>Create your admin account to get started.</p>
        <form onSubmit={submitSetup}>
          <div className="form-group"><input className="form-input" placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} autoFocus /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Password (min 8 characters)" value={password} onChange={e => setPassword(e.target.value)} /></div>
          {error && <div className="error-msg">{error}</div>}
          <button type="submit" className="btn primary" style={{ width: '100%', justifyContent: 'center', marginTop: 8 }}>Create account</button>
        </form>
      </div>
    )
  }

  if (!status.authenticated) {
    return (
      <div className="wrap token-form">
        <h1>Pulse Admin</h1>
        <p>Sign in to continue.</p>
        <form onSubmit={submitLogin}>
          <div className="form-group"><input className="form-input" placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} autoFocus /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)} /></div>
          {error && <div className="error-msg">{error}</div>}
          <button type="submit" className="btn primary" style={{ width: '100%', justifyContent: 'center', marginTop: 8 }}>Sign in</button>
        </form>
      </div>
    )
  }

  return (
    <div className="admin-layout">
      <aside className="admin-sidebar">
        <div className="admin-sidebar-title">Pulse Admin</div>
        {NAV.map(n => (
          <Link key={n.href} href={n.href} className={`admin-nav-link${pathname === n.href ? ' active' : ''}`}>{n.label}</Link>
        ))}
        <button className="admin-nav-link" style={{ border: 'none', background: 'none', cursor: 'pointer', width: '100%', textAlign: 'left', marginTop: 20, color: 'var(--down)' }} onClick={signOut}>
          Sign out{status.username ? ` (${status.username})` : ''}
        </button>
      </aside>
      <div className="admin-content">{children}</div>
    </div>
  )
}
```

- [ ] **Step 4: Verify the export builds.**
`cd /Users/shreedabhat/Documents/statuspage/pulse/ui && NEXT_PUBLIC_API_URL="" npm run build`
Expected: build succeeds (TS errors here mean a missed token-argument removal in Step 2 — fix them). Confirm `ui/out/admin/` present.

- [ ] **Step 5: YOU COMMIT** — "feat(admin): username/password setup + login + logout (cookie sessions)"

---

## Task 7: `pulse reset-password` subcommand

**Why a subcommand of `pulse` (not `pulse-cli`):** the `cli` module is a separate Go module and **cannot import `api/internal/...`** (Go's internal-package rule). The `pulse` binary lives in the `api` module, so it can use `auth` + `generated` directly. This also keeps recovery in the one shipped binary — better for the single-binary/lightweight goal. Invocation: `pulse reset-password --username admin --password <new>`.

**Files:**
- Create: `api/internal/account/account.go`, `api/internal/account/account_test.go`
- Modify: `api/cmd/pulse/main.go` (subcommand dispatch)

- [ ] **Step 1: Write the failing test** `api/internal/account/account_test.go`:
```go
package account

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestResetPasswordUpdatesHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h, _ := auth.HashPassword("old-password")
	_, _ = q.CreateUser(context.Background(), generated.CreateUserParams{Username: "admin", PasswordHash: h})

	if err := ResetPassword(context.Background(), q, "admin", "brand-new-pass"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	u, _ := q.GetUserByUsername(context.Background(), "admin")
	ok, _ := auth.VerifyPassword("brand-new-pass", u.PasswordHash)
	if !ok {
		t.Fatal("password was not updated to the new value")
	}
}
```

- [ ] **Step 2: Run test, confirm FAILS** (undefined: ResetPassword):
`cd /Users/shreedabhat/Documents/statuspage/pulse/api && go test ./internal/account/ -run TestResetPasswordUpdatesHash -v`

- [ ] **Step 3: Implement** `api/internal/account/account.go`:
```go
package account

import (
	"context"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

// ResetPassword sets a new Argon2id password hash for the given username.
func ResetPassword(ctx context.Context, q *generated.Queries, username, newPassword string) error {
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return q.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{
		PasswordHash: hash,
		Username:     username,
	})
}
```
> Confirm the generated `UpdateUserPasswordParams` field order/names from Task 1.

- [ ] **Step 4: Run test, confirm PASS.**

- [ ] **Step 5: Add the subcommand to `api/cmd/pulse/main.go`.** Before the normal server startup in `main()`, dispatch on `os.Args[1]`:
```go
	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		runResetPassword(os.Args[2:])
		return
	}
```
and add (same file or a `reset_password.go` in package `main`):
```go
func runResetPassword(args []string) {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "new password (min 8 chars)")
	_ = fs.Parse(args)
	if *username == "" || len(*password) < 8 {
		log.Fatal("usage: pulse reset-password --username <name> --password <min-8-chars>")
	}
	cfg := config.Load()
	path := cfg.SQLitePath
	if path == "" {
		path = "/data/pulse.db"
	}
	conn, err := db.Open(path)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer conn.Close()
	if err := account.ResetPassword(context.Background(), generated.New(conn), *username, *password); err != nil {
		log.Fatalf("reset-password: %v", err)
	}
	log.Printf("password updated for %q", *username)
}
```
Add imports: `flag`, `context`, `github.com/memetics19/pulse/api/internal/account`, `github.com/memetics19/pulse/api/internal/generated`.

- [ ] **Step 6: Verify build + a real reset:**
```bash
cd /Users/shreedabhat/Documents/statuspage/pulse/api && go build -o /tmp/pulse ./cmd/pulse
SQLITE_PATH=/tmp/rp.db /tmp/pulse reset-password --username nobody --password testpass123 ; echo "exit=$?"  # no such user -> non-zero, but DB migrates
rm -f /tmp/rp.db
```
Expected: builds; running against a user that doesn't exist updates 0 rows (UpdateUserPassword is `:exec`, so it succeeds with no error even if no row matches — acceptable; the happy path is covered by the unit test). The command prints the success line.

- [ ] **Step 7: YOU COMMIT** — "feat: pulse reset-password subcommand"

---

## Task 8: End-to-end verification (run both FE + BE)

- [ ] **Step 1: Build the binary with the real admin embedded.**
```bash
cd /Users/shreedabhat/Documents/statuspage/pulse && make build
```

- [ ] **Step 2: Run on a spare port with a fresh DB:**
```bash
API_PORT=8090 SQLITE_PATH=/tmp/pulse-auth.db /Users/shreedabhat/Documents/statuspage/pulse/bin/pulse &
SRV=$!; sleep 2
```

- [ ] **Step 3: Exercise the full auth flow via curl (cookie jar):**
```bash
echo "status (fresh -> needs_setup true):" && curl -s localhost:8090/api/auth/status
echo "setup:" && curl -s -i -c /tmp/jar -X POST localhost:8090/api/auth/setup -H 'Content-Type: application/json' -d '{"username":"admin","password":"supersecret"}' | grep -i 'HTTP/\|set-cookie'
echo "monitors WITHOUT cookie (expect 401):" && curl -s -o /dev/null -w '%{http_code}\n' localhost:8090/api/monitors
echo "monitors WITH cookie (expect 200):" && curl -s -o /dev/null -w '%{http_code}\n' -b /tmp/jar localhost:8090/api/monitors
echo "logout:" && curl -s -o /dev/null -w '%{http_code}\n' -b /tmp/jar -c /tmp/jar -X POST localhost:8090/api/auth/logout
echo "monitors after logout (expect 401):" && curl -s -o /dev/null -w '%{http_code}\n' -b /tmp/jar localhost:8090/api/monitors
kill $SRV 2>/dev/null; rm -f /tmp/pulse-auth.db /tmp/jar
```
Expected: status shows `needs_setup:true`; setup returns 201 + `Set-Cookie: pulse_session=...`; monitors 401 without cookie, 200 with; logout 200; monitors 401 after logout.

- [ ] **Step 4: Manual browser check.** Open `http://localhost:8090/admin/` → setup screen (create admin) → lands in admin → reload stays logged in → Sign out → login screen. Confirm no token field remains.

- [ ] **Step 5: YOU COMMIT** — "test: verify end-to-end auth flow" (if any verification helpers were added; otherwise nothing to commit).

---

## Self-Review notes (for the executor)

- **Spec coverage (§5):** setup ✅, login ✅, Argon2id ✅, sessions ✅, logout ✅, CLI reset ✅. **Deferred to Plan 2b: TOTP 2FA and API keys** (the `users` table already has `totp_secret`/`totp_enabled` so TOTP needs no further migration).
- **Signature change:** `server.New` drops `adminToken` (Task 5); the `cmd/pulse` test is updated in the same task. The old `RequireToken` + its test are removed.
- **Security note:** cookies are issued with `secure=false` for plain HTTP local/dev. When Plan 3 adds autocert/TLS, pass `secure=true` to `handlers.NewAuth` (and thread a config flag).
- **DRY:** reuse any existing `writeJSON` in `handlers` rather than the one shown.
