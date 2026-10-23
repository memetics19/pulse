# Security

This page covers how Pulse handles authentication, sessions, two-factor authentication, password recovery, and API keys. It ends with how to check that a release really came from this repository.

## Password hashing

Admin passwords are hashed with Argon2id. The plaintext password is never stored.

## Sessions

Login uses a username and password. On success, Pulse sets an HttpOnly session cookie. The HttpOnly flag prevents client-side scripts from reading the cookie.

Some changes sign out sessions, so a session opened before the change cannot outlive it. This matters when a cookie may have been stolen.

- Turning 2FA on or off signs out every other session of that user. The browser that made the change stays signed in.
- Resetting the password with `pulse reset-password` signs out every session of that user, because there is no current session on the command line.

## Two-factor authentication (TOTP)

Pulse supports optional time-based one-time password (TOTP) two-factor authentication with an authenticator app.

- **Enable.** In the admin settings, start TOTP enrollment, scan the code with an authenticator app, and confirm with a generated code. After confirmation, login requires the password and a current code.
- **Pending secret.** Starting enrollment stores a new secret, but 2FA stays off until you confirm it with a code. Login never asks for or accepts a code from an unconfirmed secret.
- **Disable.** In the admin settings, enter a current code from your authenticator app and turn TOTP off. Pulse refuses the request when the code is missing or wrong, and 2FA stays on. After disabling, login requires only the password.
- **Change authenticator.** Pulse refuses to start enrollment while 2FA is on. Running setup again can never replace or drop the active secret. To move to a new app, disable 2FA with a code from the old app, then enroll again.

These rules stop someone who only holds a session cookie, for example from an unlocked browser, from removing the second factor.

Each code works once. Pulse records the time step of the last code it accepted for each user and refuses any code from that step or an older one. This applies to login, enabling and disabling. The check and the record happen in one database update, so two requests racing with the same code cannot both succeed. A code that was already used gets a clear error. Wait for the next code in your app and try again. The code you used to turn 2FA on cannot also be used to log in or to turn it off.

The 2FA endpoints accept only a logged-in session. API keys are refused, whatever their scopes. Each account gets 10 code attempts on these endpoints, refilled at one every 6 seconds. That is the same budget login gives each IP address. Pulse compares codes in constant time.

## Resetting the admin password

If you are locked out, reset the admin password from the command line with the `reset-password` subcommand:

```bash
pulse reset-password
```

Run this against the same data directory and SQLite file the server uses so the change applies to the running configuration. The reset signs out every session of that user. It fails with an error when no user has that name.

## API key handling

API keys are named and scoped. See [API](api.md) for the scope list and Bearer usage. Key handling has these properties:

- The full key is shown once, at creation. Save it then.
- Only a hash of the key is stored. Pulse cannot show the full key again.
- Keys are revocable. Revoke a key to invalidate it immediately.
- Each key carries only the scopes you assign, so you can grant least privilege for a given integration.

## Public endpoints

The status page, its JSON at `/api/status`, and its Atom feed are public. They show only the monitors and incidents of the page that matches the request Host. Monitors appear by name and status alone. Targets, raw check errors and status codes, and incident update authors are never public.

Raw check results (`/api/monitors/{id}/checks`) and incident updates (`/api/incidents/{id}/updates`) need a session, or an API key with `monitors:read` or `incidents:read`. Pulse 3.0.0 and earlier served them to anyone. Their errors can name internal hosts and ports, so treat an older instance that was reachable from the internet as having exposed that detail. See [API](api.md#public-endpoints) for the full list of public routes.

## Verify a release

Every release is built by `.github/workflows/release.yml` in GitHub Actions. Nothing is
signed with a stored key. Signatures use Sigstore keyless signing: the certificate names
the workflow that made it, and the signature is recorded in the public Rekor transparency
log. You need [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) and
the [GitHub CLI](https://cli.github.com/).

### Images

`ghcr.io/memetics19/pulse` and `ghcr.io/memetics19/pulse-agent` are signed by digest.
Check a version like this:

```sh
cosign verify ghcr.io/memetics19/pulse:<version> \
  --certificate-identity-regexp '^https://github\.com/memetics19/pulse/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Use the same command with `pulse-agent` for the agent image. A tag points at one digest
for both architectures, so one check covers amd64 and arm64.

Each image also carries an SPDX SBOM and SLSA provenance from the build. To read them:

```sh
docker buildx imagetools inspect ghcr.io/memetics19/pulse:<version> --format '{{ json .SBOM }}'
docker buildx imagetools inspect ghcr.io/memetics19/pulse:<version> --format '{{ json .Provenance }}'
```

### SHA256SUMS

`SHA256SUMS` lists the hash of every binary, package and SBOM on the release.
`SHA256SUMS.sigstore.json` is its signature bundle. Download both, check the signature,
then check your files against the list:

```sh
BASE=https://github.com/memetics19/pulse/releases/download/v<version>
curl -fsSLO "$BASE/SHA256SUMS"
curl -fsSLO "$BASE/SHA256SUMS.sigstore.json"
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/memetics19/pulse/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c SHA256SUMS
```

On macOS use `shasum -a 256 --ignore-missing -c SHA256SUMS` for the last line.

### Build provenance

Every binary, package, SBOM and `SHA256SUMS` has a build provenance attestation. It
records the commit and the workflow run that built the file. Check any downloaded file
with:

```sh
gh attestation verify pulse_linux_amd64 --repo memetics19/pulse
```

### SBOMs

Each binary has an SPDX JSON SBOM next to it on the release, named after the binary, for
example `pulse_linux_amd64.spdx.json`. It lists the Go modules built into that binary.
