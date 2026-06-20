# Pulse Plan — TOTP 2FA Implementation Plan

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. `- [ ]` checkboxes.
> **PROJECT GIT RULE:** no git add/commit/push/mv from the agent. Stop at **YOU COMMIT**.

**Goal:** Optional authenticator-app (TOTP) 2FA for the single admin, Uptime-Kuma-style: enable from Settings (scan QR / enter code to confirm), require a 6-digit code at login when enabled, and disable from Settings.

**Architecture:** `users` already has `totp_secret *string` + `totp_enabled int64`. A `totp` helper (pquerna/otp) generates a secret + otpauth URI and validates codes; a server-side QR PNG (skip2/go-qrcode) is returned as a data URL for scanning. Session-protected `/api/auth/2fa/{setup,enable,disable}` endpoints manage it (acting on the logged-in session's user). `Login` requires a valid code when `totp_enabled==1` (responds `{needs_2fa:true}` with 401 when a code is missing, so the UI can prompt). A Settings "Two-factor authentication" card and a login code field complete it.

**Tech Stack:** Go (`github.com/pquerna/otp/totp`, `github.com/skip2/go-qrcode`), sqlc/SQLite, Next.js admin.

**Builds on:** Plan 2 (auth: `users.totp_*`, sessions, `handlers/auth.go`, `RequireSession`).

---

## Task 1: queries + totp helper
**Files:** `api/internal/db/queries/auth.sql` (add), regenerate; `api/internal/auth/totp.go` (+ test); `api/go.mod` deps.

- [ ] **Step 1** add to `auth.sql`:
```sql
-- name: SetUserTOTP :exec
UPDATE users SET totp_secret = ?, totp_enabled = ? WHERE id = ?;
```
regenerate (`cd api/internal/db && sqlc generate`); note `SetUserTOTPParams{TotpSecret *string, TotpEnabled int64, ID int64}`.
- [ ] **Step 2** `cd api && go get github.com/pquerna/otp@latest github.com/skip2/go-qrcode@latest && go mod tidy`.
- [ ] **Step 3** failing test `api/internal/auth/totp_test.go`:
```go
package auth

import (
	"testing"
	"github.com/pquerna/otp/totp"
)

func TestTOTPGenerateAndValidate(t *testing.T) {
	secret, uri, err := GenerateTOTP("admin")
	if err != nil { t.Fatal(err) }
	if secret == "" || uri == "" { t.Fatal("empty secret/uri") }
	code, err := totp.GenerateCode(secret, timeNow())
	if err != nil { t.Fatal(err) }
	if !ValidateTOTP(secret, code) { t.Fatal("freshly generated code should validate") }
	if ValidateTOTP(secret, "000000") && code != "000000" { t.Fatal("wrong code should not validate") }
	qr, err := TOTPQRDataURL(uri)
	if err != nil || len(qr) < 20 || qr[:10] != "data:image" { t.Fatalf("bad qr: %v %q", err, qr) }
}
```
> add a tiny `func timeNow() time.Time { return time.Now() }` in the test, or use `time.Now()` directly.
- [ ] **Step 4** implement `api/internal/auth/totp.go`:
```go
package auth

import (
	"bytes"
	"encoding/base64"

	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"
)

const totpIssuer = "Pulse"

// GenerateTOTP creates a new secret + otpauth provisioning URI for the account.
func GenerateTOTP(account string) (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: account})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTP reports whether code is currently valid for secret.
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// TOTPQRDataURL renders the provisioning URI as a PNG data URL for scanning.
func TOTPQRDataURL(uri string) (string, error) {
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	b.WriteString("data:image/png;base64,")
	b.WriteString(base64.StdEncoding.EncodeToString(png))
	return b.String(), nil
}
```
- [ ] **Step 5** run → PASS; `go build ./...`. **Step 6: YOU COMMIT** — "feat(auth): TOTP helper + QR"

---

## Task 2: 2FA handlers + login enforcement
**Files:** `api/internal/handlers/auth.go` (+ a `twofa.go` or extend auth); test; `server.go` (routes).

- [ ] **Step 1** Add a helper to resolve the current session's user in handlers: `func (a *Auth) currentUser(r) (generated.User, bool)` — read cookie → `GetSession` → `GetUserByID`. (Reuse if a similar exists.)
- [ ] **Step 2** Handlers (methods on `Auth`, session-protected):
  - `TwoFASetup`: current user; `auth.GenerateTOTP(user.Username)`; store secret via `SetUserTOTP(secret, 0, user.ID)` (not enabled yet); respond `{secret, otpauth_url, qr}` (qr = `TOTPQRDataURL`).
  - `TwoFAEnable`: body `{code}`; current user; if `user.TotpSecret==nil` → 400; if `!auth.ValidateTOTP(*user.TotpSecret, code)` → 400 "invalid code"; else `SetUserTOTP(user.TotpSecret, 1, user.ID)`; 200.
  - `TwoFADisable`: current user; `SetUserTOTP(nil, 0, user.ID)`; 200.
- [ ] **Step 3** Modify `Login`: after password verifies, if `u.TotpEnabled == 1`: read `code` from the login body; if empty → respond `writeJSON(w, 401, {"needs_2fa": true})`; if `!auth.ValidateTOTP(*u.TotpSecret, code)` → 401 "invalid 2FA code"; else proceed to `startSession`. (Add `Code string \`json:"code"\`` to the credentials struct.)
- [ ] **Step 4** Modify `Status`: include `"totp_enabled": <bool>` for the authenticated user (so Settings can show state). Reuse the session→user lookup.
- [ ] **Step 5** test `auth_test.go` (extend): setup → enable with a generated code → login without code returns 401+needs_2fa → login with a valid code succeeds. (generate codes via `totp.GenerateCode(secret, time.Now())`.)
- [ ] **Step 6** `server.go`: add to the **session-only** group: `r.Post("/api/auth/2fa/setup", authH.TwoFASetup)`, `r.Post("/api/auth/2fa/enable", authH.TwoFAEnable)`, `r.Post("/api/auth/2fa/disable", authH.TwoFADisable)`. (Login/logout/status stay public.)
- [ ] **Step 7** `go build ./... && go test ./... -count=1` green. **Step 8: YOU COMMIT** — "feat(auth): 2FA setup/enable/disable + login enforcement"

---

## Task 3: admin UI — Settings 2FA + login code
**Files:** `ui/src/lib/api.ts`, `ui/src/app/admin/settings/page.tsx`, `ui/src/app/admin/layout.tsx` (login form), `AuthStatus` type.

- [ ] **Step 1** `api.ts`: `twoFASetup()` → `{secret, otpauth_url, qr}`; `twoFAEnable(code)`; `twoFADisable()`. Extend `authLogin` to accept an optional `code` and surface a `needs_2fa` signal (e.g., return `'needs_2fa'` or throw a typed error). Add `totp_enabled` to `AuthStatus`.
- [ ] **Step 2** Settings page: a **"Two-factor authentication"** `.card`. If `!totp_enabled`: an "Enable 2FA" button → calls `twoFASetup()` → shows the **QR image** (`<img src={qr}>`) + the secret (mono, copyable) + a 6-digit code input + "Verify & enable" (→ `twoFAEnable(code)`, refresh). If `totp_enabled`: show "Two-factor authentication is on" + a "Disable" button (→ `twoFADisable`). (Fetch status via `authStatus()`.)
- [ ] **Step 3** Login form (`layout.tsx`): on submit, call `authLogin(username, password)`; if it signals `needs_2fa`, reveal a 6-digit **Authenticator code** input and resubmit with the code. Keep the form minimal/v3.
- [ ] **Step 4** `cd ui && NEXT_PUBLIC_API_URL="" npm run build` → succeeds. **Step 5: YOU COMMIT** — "feat(admin): 2FA settings + login code"

---

## Task 4: end-to-end verification
- [ ] Build + run; setup an admin; `POST /api/auth/2fa/setup` (with session) → returns secret+qr; generate a code from the secret (a tiny Go snippet or `oathtool`); `POST /api/auth/2fa/enable {code}` → 200; logout; `POST /api/auth/login {username,password}` → 401 `needs_2fa`; `POST /api/auth/login {username,password,code}` → 200 + cookie; `POST /api/auth/2fa/disable` → login without code works again.
- [ ] **YOU COMMIT** — "test: 2FA end-to-end"

---

## Self-Review
- Security: secret stored only server-side; enabled only after a verified code; login requires a valid code when enabled; disable clears the secret. pquerna/otp is the standard TOTP lib.
- Consistency: `SetUserTOTP` param names per Task 1; `needs_2fa` signal shared between Login handler and the UI; `totp_enabled` surfaced in `AuthStatus` + Settings.
