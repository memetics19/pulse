# Pulse Plan 2b — API Keys Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. `- [ ]` checkboxes.
> **PROJECT GIT RULE:** no git add/commit/push/mv from the agent. Stop at **YOU COMMIT**.

**Goal:** Named, revocable API keys for programmatic REST access, with granular scopes, shown once on creation, accepted via `Authorization: Bearer pulse_…` as an alternative to the admin session cookie.

**Architecture:** New `api_keys` table (sha256 hash of the key + visible prefix + JSON scopes). A `keyauth` helper generates `pulse_live_<rand>` keys and hashes them. CRUD handlers under `/api/keys` (session-only). A combined middleware `RequireSessionOrAPIKey(q)` replaces `RequireSession` on the API admin group: a valid session cookie grants full access (unchanged); otherwise a valid, unrevoked API key is checked against the **scope required for that method+path** and its `last_used_at` is touched. An admin **API Keys** page lists keys (name, prefix, scopes, last used) and creates/revokes them — the full key is displayed once.

**Tech Stack:** Go (chi, crypto/rand, crypto/sha256), sqlc/SQLite, Next.js admin SPA.

**Builds on:** Plan 2 (`auth`, sessions, `RequireSession`), Plan 2c (`app.App`, server wiring via `app.LiveDBTX`).

**Scopes (offered at creation):** `monitors:read`, `monitors:write`, `incidents:read`, `incidents:write`, `notifications:read`, `notifications:write`, `agents:read`, `agents:write`, `theme:read`, `theme:write`, `status:read`. Key management (`/api/keys`), auth, and setup are **session-only** (never reachable with an API key).

**Path → required scope (for API-key requests):**
| Path prefix | GET | mutate (POST/PUT/DELETE) |
|---|---|---|
| `/api/monitors`, `/api/groups` | `monitors:read` | `monitors:write` |
| `/api/incidents` | `incidents:read` | `incidents:write` |
| `/api/notifications` | `notifications:read` | `notifications:write` |
| `/api/agents` | `agents:read` | `agents:write` |
| `/api/theme` | `theme:read` | `theme:write` |
| `/api/overview` | `status:read` | — |

---

## Task 1: api_keys schema + queries

**Files:** `api/internal/db/migrations/4_api_keys.{up,down}.sql`, `api/internal/db/queries/api_keys.sql`, regenerate.

- [ ] **Step 1** `4_api_keys.up.sql`:
```sql
CREATE TABLE api_keys (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    key_hash     TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    scopes       TEXT NOT NULL DEFAULT '[]',
    last_used_at TIMESTAMP,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at   TIMESTAMP
);
```
- [ ] **Step 2** `4_api_keys.down.sql`: `DROP TABLE api_keys;`
- [ ] **Step 3** `api_keys.sql`:
```sql
-- name: CreateAPIKey :one
INSERT INTO api_keys (name, key_hash, prefix, scopes) VALUES (?, ?, ?, ?) RETURNING *;

-- name: ListAPIKeys :many
SELECT * FROM api_keys ORDER BY created_at DESC;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = ? WHERE id = ?;
```
- [ ] **Step 4** regenerate (`cd api/internal/db && sqlc generate`), `go build ./...`. Note generated `ApiKey` fields + `CreateAPIKeyParams` (Name, KeyHash, Prefix, Scopes), `TouchAPIKeyParams` (LastUsedAt *time.Time, ID).
- [ ] **Step 5** migration smoke test in `db_test.go` (insert + select an api_keys row) → PASS.
- [ ] **Step 6: YOU COMMIT** — "feat(db): api_keys schema + queries"

---

## Task 2: key generation + hashing

**Files:** `api/internal/keyauth/keyauth.go` + test.

- [ ] **Step 1: failing test** `keyauth_test.go`:
```go
package keyauth

import "testing"

func TestGenerateAndHash(t *testing.T) {
	full, prefix, hash, err := Generate()
	if err != nil { t.Fatal(err) }
	if len(full) < 20 || prefix == "" || hash == "" {
		t.Fatalf("bad outputs: full=%q prefix=%q", full, prefix)
	}
	if Hash(full) != hash {
		t.Fatal("Hash(full) must equal the returned hash")
	}
	if !hasPrefix(full) {
		t.Fatalf("key should start with the pulse prefix: %q", full)
	}
}
func hasPrefix(s string) bool { return len(s) > 10 && s[:10] == "pulse_live" }
```
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3: implement** `keyauth.go`:
```go
package keyauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const keyPrefix = "pulse_live_"

// Generate returns (fullKey, displayPrefix, sha256hash). The full key is shown
// to the user once; only the hash is stored.
func Generate() (full, prefix, hash string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(b)
	full = keyPrefix + secret
	prefix = full[:len(keyPrefix)+6] // e.g. pulse_live_ab12cd
	hash = Hash(full)
	return full, prefix, hash, nil
}

// Hash returns the hex sha256 of a key (API keys are high-entropy, so a fast
// hash is appropriate — unlike user passwords).
func Hash(full string) string {
	sum := sha256.Sum256([]byte(full))
	return hex.EncodeToString(sum[:])
}
```
- [ ] **Step 4** run → PASS. **Step 5: YOU COMMIT** — "feat(keyauth): API key generation + hashing"

---

## Task 3: API-key CRUD handlers

**Files:** `api/internal/handlers/apikeys.go` + test.

- [ ] **Step 1: failing test** `apikeys_test.go`: create a key (POST body `{name, scopes:[...]}`) → 201 with a `key` field (the full key, starts with `pulse_live_`) and it is NOT returned again by list; list returns the key with `prefix` but no full key; revoke → 204/200.
```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestAPIKeyCreateListRevoke(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	h := NewAPIKeys(q)

	body, _ := json.Marshal(map[string]any{"name": "ci", "scopes": []string{"monitors:read"}})
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated { t.Fatalf("create=%d", rec.Code) }
	var created struct{ Key string `json:"key"` }
	json.NewDecoder(rec.Body).Decode(&created)
	if !strings.HasPrefix(created.Key, "pulse_live_") { t.Fatalf("missing full key: %q", created.Key) }

	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/keys", nil))
	if strings.Contains(rec.Body.String(), created.Key) { t.Fatal("list must NOT contain the full key") }
}
```
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3: implement** `apikeys.go`: `NewAPIKeys(q)`; `Create` (decode {name, scopes []string}; `keyauth.Generate()`; store via `CreateAPIKey` with `scopes` JSON-marshaled; respond 201 `{id, name, prefix, scopes, key: <full>}` — full key ONLY here); `List` (return rows mapped to `{id,name,prefix,scopes,last_used_at,created_at}` — never the hash/full key); `Revoke` (path id → `RevokeAPIKey`). Reuse `writeJSON`. Marshal/unmarshal `scopes` to/from the JSON text column.
- [ ] **Step 4** run → PASS, `go build ./...`. **Step 5: YOU COMMIT** — "feat(api): API key CRUD handlers"

---

## Task 4: combined session-or-API-key middleware

**Files:** `api/internal/middleware/apikey.go` + test. Modify `api/internal/middleware/session.go` only if needed.

- [ ] **Step 1: failing test** `apikey_test.go`: build `RequireSessionOrAPIKey(q)`; (a) no cred → 401; (b) valid session cookie → 200 (full); (c) valid API key with `monitors:read` on a GET `/api/monitors` → 200; (d) same key on POST `/api/monitors` (needs `monitors:write`) → 403; (e) revoked key → 401. (Seed keys via `keyauth.Generate` + `CreateAPIKey`.)
- [ ] **Step 2** run → FAIL.
- [ ] **Step 3: implement** `apikey.go`:
```go
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

// requiredScope maps an API request to the scope an API key must hold.
// Returns "" if API keys may not access this path (session-only).
func requiredScope(method, path string) string {
	resource := ""
	switch {
	case strings.HasPrefix(path, "/api/monitors"), strings.HasPrefix(path, "/api/groups"):
		resource = "monitors"
	case strings.HasPrefix(path, "/api/incidents"):
		resource = "incidents"
	case strings.HasPrefix(path, "/api/notifications"):
		resource = "notifications"
	case strings.HasPrefix(path, "/api/agents"):
		resource = "agents"
	case strings.HasPrefix(path, "/api/theme"):
		resource = "theme"
	case strings.HasPrefix(path, "/api/overview"):
		return "status:read" // read-only resource
	default:
		return "" // /api/keys, /api/auth, /api/setup → session only
	}
	if method == http.MethodGet {
		return resource + ":read"
	}
	return resource + ":write"
}

func RequireSessionOrAPIKey(q *generated.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Session cookie (full access, unchanged behaviour).
			if c, err := r.Cookie(auth.SessionCookieName); err == nil {
				if sess, err := q.GetSession(r.Context(), c.Value); err == nil && sess.ExpiresAt.After(time.Now()) {
					next.ServeHTTP(w, r)
					return
				}
			}
			// 2. API key.
			authz := r.Header.Get("Authorization")
			if strings.HasPrefix(authz, "Bearer pulse_") {
				token := strings.TrimPrefix(authz, "Bearer ")
				key, err := q.GetAPIKeyByHash(r.Context(), keyauth.Hash(token))
				if err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				need := requiredScope(r.Method, r.URL.Path)
				if need == "" || !scopesContain(key.Scopes, need) {
					http.Error(w, "insufficient scope", http.StatusForbidden)
					return
				}
				now := time.Now()
				_ = q.TouchAPIKey(r.Context(), generated.TouchAPIKeyParams{LastUsedAt: &now, ID: key.ID})
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

func scopesContain(scopesJSON, need string) bool {
	// scopes stored as JSON array text; simple substring match on the quoted scope is sufficient
	return strings.Contains(scopesJSON, `"`+need+`"`)
}
```
> Confirm `key.Scopes` is the column (string) and `key.ID` exist on the generated `ApiKey`. Adjust `TouchAPIKeyParams` field names to the generated ones.
- [ ] **Step 4** run → PASS. **Step 5: YOU COMMIT** — "feat(auth): session-or-API-key middleware with scopes"

---

## Task 5: wire routes + middleware

**Files:** `api/internal/server/server.go`.

- [ ] **Step 1** In the admin API group, change `r.Use(middleware.RequireSession(q))` to `r.Use(middleware.RequireSessionOrAPIKey(q))`.
- [ ] **Step 2** Add a **session-only** sub-group (still `RequireSession`) for key management, OR register `/api/keys` routes that explicitly use `RequireSession(q)` (NOT the combined one — an API key must not manage keys):
```go
r.Group(func(r chi.Router) {
    r.Use(middleware.RequireSession(q))
    kh := handlers.NewAPIKeys(q)
    r.Get("/api/keys", kh.List)
    r.Post("/api/keys", kh.Create)
    r.Delete("/api/keys/{id}", kh.Revoke)
})
```
- [ ] **Step 3** `go build ./... && go test ./... -count=1` → green. **Step 4: YOU COMMIT** — "feat: wire API keys + scoped auth"

---

## Task 6: admin API Keys page

**Files:** `ui/src/app/admin/api-keys/page.tsx` (new), `ui/src/app/admin/layout.tsx` (nav), `ui/src/lib/api.ts` (client fns).

- [ ] **Step 1** `api.ts`: `listApiKeys()`, `createApiKey(name, scopes: string[])` (returns `{key, ...}`), `revokeApiKey(id)` — all `credentials:'include'`.
- [ ] **Step 2** Add `{ href: '/admin/api-keys', label: 'API Keys' }` to `NAV` (after Notifications).
- [ ] **Step 3** Create `ui/src/app/admin/api-keys/page.tsx` (`'use client'`, v3 look): a `.card` table of keys (name, prefix `pulse_live_…`, scopes, last used, created, Revoke with inline confirm). A "New API key" modal: name + a checkbox list of the scopes (`monitors:read`, `monitors:write`, `incidents:read`, `incidents:write`, `notifications:read`, `notifications:write`, `agents:read`, `agents:write`, `theme:read`, `theme:write`, `status:read`). On create, show the **full key once** in a highlighted box with a "Copy" button and a "you won't see this again" note; list refreshes after.
- [ ] **Step 4** `cd ui && NEXT_PUBLIC_API_URL="" npm run build` → succeeds; `ui/out/admin/api-keys/` present.
- [ ] **Step 5: YOU COMMIT** — "feat(admin): API Keys page"

---

## Self-Review
- Spec coverage: named/revocable keys ✅, hash-stored + shown-once ✅, granular scopes ✅, `Bearer` + scope-checked middleware ✅, key mgmt session-only ✅, admin UI ✅. (Maintenance scopes omitted — maintenance isn't built yet; add `maintenance:*` when it lands.)
- Consistency: scope strings identical across the create UI (Task 6), the `requiredScope` map (Task 4), and the docs table. `keyauth.Hash` used in both create and verify.
- Security: only sha256 hash stored; full key shown once; revoked keys rejected (`GetAPIKeyByHash` filters `revoked_at IS NULL`); API keys cannot reach `/api/keys`, auth, or setup.
