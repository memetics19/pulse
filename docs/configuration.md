# Configuration

Pulse is configured with environment variables. Notification channels are optional and stay disabled when their variables are unset.

## Environment variables

Every server setting starts with `PULSE_`, apart from the two notification keys.

| Variable | Default | Purpose |
| --- | --- | --- |
| `PULSE_PORT` | `8080` | TCP port the HTTP server listens on. |
| `PULSE_DB` | `/data/pulse.db` | Full path of the SQLite database file. See [Where the database lives](#where-the-database-lives). |
| `PULSE_DATA_DIR` | `/data` | Directory for the setup state (`pulse.json`). When `PULSE_DB` is unset, Pulse creates `pulse.db` here. The container image sets it to `/data`. |
| `PULSE_DATABASE_URL` | (unset) | Reserved for an external database. Pulse reads it but stores everything in SQLite. The unprefixed `DATABASE_URL` is not read. |
| `RESEND_API_KEY` | (unset) | API key for sending email notifications through Resend. Leave unset to disable email. |
| `PULSE_EMAIL_FROM` | (unset) | Sender of email alerts, such as `alerts@example.com` or `Pulse <alerts@example.com>`. It must be on a domain verified in the same Resend account. Resend refuses any other. With `RESEND_API_KEY` set and this unset, Pulse logs a warning at startup and every email alert fails with the reason in the log. |
| `SLACK_WEBHOOK_URL` | (unset) | Slack incoming webhook URL for notifications. Leave unset to disable Slack. |
| `PULSE_SECURE_COOKIES` | `false` | Set to `true` when Pulse is served over HTTPS to mark session cookies `Secure`. |
| `PULSE_CORS_ORIGINS` | (unset) | Comma-separated list of origins allowed to call the API cross-origin. Leave it unset (the default) to send no CORS headers. The embedded UI is served from the same origin and needs none. |
| `PULSE_ALLOW_PRIVATE_MONITORS` | `false` | Allow HTTP(S) monitors to target private/internal addresses (loopback, LAN, link-local). **Set to `true` for homelab setups that monitor LAN services.** Off by default so monitor URLs cannot be used to probe the server's internal network (SSRF). |
| `PULSE_TRUSTED_PROXIES` | (unset) | Comma-separated CIDRs or IPs of reverse proxies whose `X-Forwarded-For` header the login rate limiter may trust. Set it when Pulse runs behind a proxy such as Caddy. Leave it unset to ignore proxy headers. |
| `PULSE_METRICS` | `false` | Set to `true` to serve Prometheus metrics at `/metrics`. Off by default. See [Metrics](metrics.md). |
| `PULSE_METRICS_TOKEN` | (unset) | When set, `/metrics` requires the header `Authorization: Bearer <token>`. Leave unset to serve metrics without a token. |

Set these in the `environment` block of `docker-compose.yml` or in your container runtime.

```yaml
environment:
  PULSE_DB: /data/pulse.db
  PULSE_PORT: "8080"
  RESEND_API_KEY: ${RESEND_API_KEY:-}
  PULSE_EMAIL_FROM: ${PULSE_EMAIL_FROM:-}
  SLACK_WEBHOOK_URL: ${SLACK_WEBHOOK_URL:-}
```

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

The setup wizard creates the database file and records its path in `pulse.json`. When `PULSE_DB` is set, the wizard shows that path and does not let you change it. The server and `pulse reset-password` both open the recorded file, so they always use the same database.

The shipped setups resolve to these files:

| Setup | Settings | Database file |
| --- | --- | --- |
| `.deb` and `.rpm` packages | `PULSE_DB=/var/lib/pulse/pulse.db`, `PULSE_DATA_DIR=/var/lib/pulse` | `/var/lib/pulse/pulse.db` |
| Container image | `PULSE_DATA_DIR=/data` | `/data/pulse.db` |
| Compose files | `PULSE_DB=/data/pulse.db` and the image's `PULSE_DATA_DIR=/data` | `/data/pulse.db` |

After setup, Pulse keeps using the recorded file, so an upgrade never moves a database. If `PULSE_DB` later names a different file, Pulse logs a warning at startup and still opens the recorded one. To move the database, stop Pulse, move the file, and set `sqlite_path` in `pulse.json` to the new path.

Before 3.0, the wizard used only the directory of `SQLITE_PATH` and always created `pulse.db` there. An install set up with `SQLITE_PATH=/srv/pulse/custom.db` keeps its data in `/srv/pulse/pulse.db`. It goes on using that file and logs the warning above until you set `PULSE_DB=/srv/pulse/pulse.db`.

## Data persistence

Pulse stores all state in the SQLite database file. Mount a persistent volume so the file survives container restarts and upgrades. In the default Compose setup, the named volume `pulse_data` is mounted at `/data`, and `PULSE_DB` points to `/data/pulse.db` on that volume.

```yaml
volumes:
  - pulse_data:/data
```

To back up Pulse, stop the container and copy the SQLite file and `pulse.json` from the volume.

## The SQLite file

Pulse uses a single SQLite file as its only data store. It holds the admin account, monitors and groups, check history, incidents and updates, maintenance windows, status pages, themes, API keys, and notification settings.

Raw check history is pruned by the worker according to the [data retention](monitors.md#data-retention) window, which defaults to 90 days.
