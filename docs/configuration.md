# Configuration

Pulse is configured with environment variables. Notification channels are optional and stay disabled when their variables are unset.

## Environment variables

Every server setting starts with `PULSE_`, apart from the two notification keys.

| Variable | Default | Purpose |
| --- | --- | --- |
| `PULSE_PORT` | `8080` | TCP port the HTTP server listens on. |
| `PULSE_DB` | `/data/pulse.db` | Full path of the SQLite database file. See [Where the database lives](#where-the-database-lives). |
| `PULSE_DATA_DIR` | `/data` | Directory for the setup state (`pulse.json`). When `PULSE_DB` is unset, Pulse creates `pulse.db` here. The container image sets it to `/data`. |
| `PULSE_SETUP_TOKEN` | (unset) | The token the first-run setup asks for. When unset, Pulse generates a random token at startup and prints it in the log. Set it to `off` only for an automated install that completes setup itself. See [Setup token](#setup-token). |
| `PULSE_DATABASE_URL` | (unset) | Reserved for an external database. Pulse reads it but stores everything in SQLite. The unprefixed `DATABASE_URL` is not read. |
| `RESEND_API_KEY` | (unset) | API key for sending email notifications through Resend. Leave it and `PULSE_SMTP_HOST` unset to disable email. Ignored when `PULSE_SMTP_HOST` is set. |
| `PULSE_EMAIL_FROM` | (unset) | Sender of email alerts, such as `alerts@example.com` or `Pulse <alerts@example.com>`. With Resend it must be on a domain verified in the same Resend account. Resend refuses any other. With email set up and this unset, Pulse logs a warning at startup and every email alert fails with the reason in the log. |
| `PULSE_SMTP_HOST` | (unset) | Host name of an SMTP relay for email alerts, such as `smtp.example.com`. Setting it sends email through SMTP instead of Resend. See [Email over SMTP](#email-over-smtp). |
| `PULSE_SMTP_PORT` | by TLS mode | Port of the relay. The default is `587` for `starttls`, `465` for `tls` and `25` for `none`. |
| `PULSE_SMTP_USER` | (unset) | User name for SMTP AUTH. Leave unset for a relay that takes mail without a login. |
| `PULSE_SMTP_PASSWORD` | (unset) | Password for SMTP AUTH. Pulse never logs it. |
| `PULSE_SMTP_PASSWORD_FILE` | (unset) | Path of a file that holds the SMTP password, such as a Docker or systemd secret. Read only when `PULSE_SMTP_PASSWORD` is unset. One trailing line break is dropped. |
| `PULSE_SMTP_TLS` | `starttls` | How the connection to the relay is encrypted: `starttls`, `tls` (TLS from the first byte) or `none`. `none` is only allowed when `PULSE_SMTP_HOST` is `localhost` or a loopback address. |
| `SLACK_WEBHOOK_URL` | (unset) | Slack incoming webhook URL for notifications. Leave unset to disable Slack. |
| `PULSE_SECURE_COOKIES` | `false` | Set to `true` when Pulse is served over HTTPS to mark session cookies `Secure`. |
| `PULSE_CORS_ORIGINS` | (unset) | Comma-separated list of origins allowed to call the API cross-origin. Leave it unset (the default) to send no CORS headers. The embedded UI is served from the same origin and needs none. |
| `PULSE_ALLOW_PRIVATE_MONITORS` | `false` | Allow HTTP(S) monitors to target private/internal addresses (loopback, LAN, link-local). **Set to `true` for homelab setups that monitor LAN services.** Off by default so monitor URLs cannot be used to probe the server's internal network (SSRF). |
| `PULSE_TRUSTED_PROXIES` | (unset) | Comma-separated CIDRs or IPs of reverse proxies whose `X-Forwarded-For` header the login rate limiter may trust. Set it when Pulse runs behind a proxy such as Caddy. Leave it unset to ignore proxy headers. |
| `PULSE_METRICS` | `false` | Set to `true` to serve Prometheus metrics at `/metrics`. Off by default. See [Metrics](metrics.md). |
| `PULSE_METRICS_TOKEN` | (unset) | When set, `/metrics` requires the header `Authorization: Bearer <token>`. Leave unset to serve metrics without a token. |
| `PULSE_BACKUP_DIR` | (unset) | Directory for automatic daily snapshots of the database. Leave unset to turn them off. See [Automatic backups](deployment.md#automatic-backups). |
| `PULSE_BACKUP_KEEP` | `7` | How many automatic snapshots to keep in `PULSE_BACKUP_DIR`. Older ones are removed. A value that is not a positive whole number is logged and `7` is used. |

Set these in the `environment` block of `docker-compose.yml` or in your container runtime.

```yaml
environment:
  PULSE_DB: /data/pulse.db
  PULSE_PORT: "8080"
  RESEND_API_KEY: ${RESEND_API_KEY:-}
  PULSE_EMAIL_FROM: ${PULSE_EMAIL_FROM:-}
  SLACK_WEBHOOK_URL: ${SLACK_WEBHOOK_URL:-}
```

## Email over SMTP

Pulse can send email alerts through any SMTP relay. Examples are Postfix, Mailcow, Proton Bridge, or a Gmail or Microsoft 365 app password. No Resend account is needed.

```sh
PULSE_SMTP_HOST=smtp.example.com
PULSE_SMTP_PORT=587
PULSE_SMTP_USER=alerts@example.com
PULSE_SMTP_PASSWORD_FILE=/run/secrets/smtp_password
PULSE_SMTP_TLS=starttls
PULSE_EMAIL_FROM="Pulse <alerts@example.com>"
```

The email is the same as with Resend. It has an HTML part, with every field escaped, and a plain text part.

- `starttls` connects in plain text and then upgrades with STARTTLS. If the relay does not offer STARTTLS, the send fails. Pulse never falls back to plain text.
- `tls` speaks TLS from the first byte. This is usually port 465.
- `none` sends without encryption. It is only allowed for a relay on `localhost`, such as a local Postfix.

Pulse checks the relay's certificate against the system's trusted roots. A certificate that does not verify fails the send. Each send has a 30 second limit for the whole session.

The relay goes through the same network guard as webhooks. A relay on a private or loopback address, such as `localhost` or a LAN host, needs `PULSE_ALLOW_PRIVATE_MONITORS=true`.

### SMTP and Resend together

When `PULSE_SMTP_HOST` and `RESEND_API_KEY` are both set, email goes through SMTP and the Resend key is ignored. Pulse logs this once at startup. SMTP wins because it is the setting you add on purpose for a self-hosted relay. A Resend key left over from an older setup should not decide where mail goes. Unset `PULSE_SMTP_HOST` to go back to Resend.

A setting that cannot work, such as an unknown `PULSE_SMTP_TLS` value or a user without a password, is logged at startup. Every email alert then fails with the same reason in the log.

## Renamed in 3.0

Pulse 3.0 gives every server setting a `PULSE_` name. Three settings changed:

| Old name | New name |
| --- | --- |
| `API_PORT` | `PULSE_PORT` |
| `SQLITE_PATH` | `PULSE_DB` |
| `DATABASE_URL` | `PULSE_DATABASE_URL` |

`API_PORT` and `SQLITE_PATH` still work in 3.x. Pulse reads an old name only when the new one is unset, and the new name wins when both are set. Each old name in use logs one warning at startup:

```text
API_PORT is deprecated, use PULSE_PORT; the old name stops working in 4.0
```

Rename the variables in your Compose file, `/etc/pulse/pulse.env` or container runtime before you upgrade to 4.0. An env file kept from an earlier `.deb` install or from the installer still has the old names.

Pulse does not read `DATABASE_URL` at all. Hosting platforms such as Heroku, Railway and Dokku set it on their own, so reading it gave a false deprecation warning on every start. Nothing uses the value, because Pulse stores everything in SQLite. Use `PULSE_DATABASE_URL` if you set it on purpose.

## Where the database lives

Pulse picks the SQLite file in this order:

1. `PULSE_DB`, the full path of the file. The old name `SQLITE_PATH` works the same way.
2. `pulse.db` in `PULSE_DATA_DIR`.
3. `/data/pulse.db`.

The setup state, `pulse.json`, lives in `PULSE_DATA_DIR`. When that is unset, it lives in the directory of the `PULSE_DB` file, and otherwise in `/data`.

Only the server decides the database file. The setup wizard shows the path as information, creates the file and records its path in `pulse.json`. A setup request cannot name a different file. To use another file, change `PULSE_DB` or `PULSE_DATA_DIR` and restart Pulse before you finish setup. The server and `pulse reset-password` both open the recorded file, so they always use the same database.

The shipped setups resolve to these files:

| Setup | Settings | Database file |
| --- | --- | --- |
| `.deb` and `.rpm` packages | `PULSE_DB=/var/lib/pulse/pulse.db`, `PULSE_DATA_DIR=/var/lib/pulse` | `/var/lib/pulse/pulse.db` |
| Container image | `PULSE_DATA_DIR=/data` | `/data/pulse.db` |
| Compose files | `PULSE_DB=/data/pulse.db` and the image's `PULSE_DATA_DIR=/data` | `/data/pulse.db` |

After setup, Pulse keeps using the recorded file, so an upgrade never moves a database. This includes a file chosen in the wizard by an earlier release, when the wizard still had a path field. If `PULSE_DB` later names a different file, Pulse logs a warning at startup and still opens the recorded one. To move the database, stop Pulse, move the file, and set `sqlite_path` in `pulse.json` to the new path.

Before 3.0, the wizard used only the directory of `SQLITE_PATH` and always created `pulse.db` there. An install set up with `SQLITE_PATH=/srv/pulse/custom.db` keeps its data in `/srv/pulse/pulse.db`. It goes on using that file and logs the warning above until you set `PULSE_DB=/srv/pulse/pulse.db`.

### When pulse.json is missing

`pulse.json` can go missing while the database stays. This happens when the file is deleted, when a backup is restored without it, or when `PULSE_DATA_DIR` changes while `PULSE_DB` still names the old file.

At startup, if there is no `pulse.json` and the database file the server would set up already has an admin, Pulse adopts that file. It writes a new `pulse.json` that records the file, starts normally, and logs a line like `adopted the database /data/pulse.db, which already has an admin`. Log in with an existing account. If you lost its password, use `pulse reset-password`.

Pulse only adopts the file named by `PULSE_DB` or `PULSE_DATA_DIR`. It never creates an account while adopting. A database file without users, or no file at all, still starts the setup wizard. If Pulse cannot write `pulse.json`, it logs that, serves the database anyway, and adopts it again on the next start.

Setup writes `pulse.json` only after the admin account is saved in the database. A setup that fails leaves no `pulse.json` behind, so you can run it again.

## Setup token

A fresh instance accepts its first setup from anyone who can reach it. The setup token makes sure that person is you, even when the instance is on a public address before you open it.

- When `PULSE_SETUP_TOKEN` is unset, Pulse generates a random token at startup while setup is not done. It prints the token once in the log, in a line like `setup token: open /setup and enter this token: ...`. Each start prints a new token, so use the most recent one.
- When `PULSE_SETUP_TOKEN` is set, setup requires that value. Pulse does not print it. Use a long random value.
- When `PULSE_SETUP_TOKEN=off`, setup needs no token. Pulse logs a warning at startup. Use this only for an automated install that runs setup itself, on an address nobody else can reach.

The wizard shows a token field when the server needs one. An automated install sends the token as `setup_token` in the `POST /api/setup` body. Pulse compares tokens in constant time. Once setup is done the token is no longer needed or accepted, and an install that is already set up prints no token.

To find the generated token:

```bash
# .deb, .rpm or installer with systemd
sudo journalctl -u pulse | grep "setup token" | tail -n 1

# Docker Compose
docker compose logs pulse | grep "setup token" | tail -n 1
```

## Data persistence

Pulse stores all state in the SQLite database file. Mount a persistent volume so the file survives container restarts and upgrades. In the default Compose setup, the named volume `pulse_data` is mounted at `/data`, and `PULSE_DB` points to `/data/pulse.db` on that volume.

```yaml
volumes:
  - pulse_data:/data
```

To back up Pulse, run `pulse backup`. It works while Pulse runs, so you do not need to stop the container. Do not copy the files of a running instance by hand. See [Backup and restore](deployment.md#backup-and-restore).

## The SQLite file

Pulse uses a single SQLite file as its only data store. It holds the admin account, monitors and groups, check history, incidents and updates, maintenance windows, status pages, themes, API keys, and notification settings.

Raw check history is pruned by the worker according to the [data retention](monitors.md#data-retention) window, which defaults to 90 days.
