# Configuration

Pulse is configured with environment variables. Notification channels are optional and stay disabled when their variables are unset.

## Environment variables

Every server setting starts with `PULSE_`, apart from the two notification keys.

| Variable | Default | Purpose |
| --- | --- | --- |
| `PULSE_PORT` | `8080` | TCP port the HTTP server listens on. |
| `PULSE_DB` | `/data/pulse.db` | Path to the SQLite database file. When `PULSE_DATA_DIR` is unset, the directory of this file is the data directory. |
| `PULSE_DATA_DIR` | `/data` | Directory for Pulse data, such as the setup state and the SQLite file. The container image sets it to `/data`. |
| `PULSE_DATABASE_URL` | (unset) | Reserved for an external database. Pulse reads it but stores everything in SQLite. |
| `RESEND_API_KEY` | (unset) | API key for sending email notifications through Resend. Leave unset to disable email. |
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
  SLACK_WEBHOOK_URL: ${SLACK_WEBHOOK_URL:-}
```

## Renamed in 3.0

Pulse 3.0 gives every server setting a `PULSE_` name. Three settings changed:

| Old name | New name |
| --- | --- |
| `API_PORT` | `PULSE_PORT` |
| `SQLITE_PATH` | `PULSE_DB` |
| `DATABASE_URL` | `PULSE_DATABASE_URL` |

The old names still work in 3.x. Pulse reads an old name only when the new one is unset, and the new name wins when both are set. Each old name in use logs one warning at startup:

```text
API_PORT is deprecated, use PULSE_PORT; the old name stops working in 4.0
```

Rename the variables in your Compose file, `/etc/pulse/pulse.env` or container runtime before you upgrade to 4.0. An env file kept from an earlier `.deb` install or from the installer still has the old names.

## Data persistence

Pulse stores all state in the SQLite database file. Mount a persistent volume so the file survives container restarts and upgrades. In the default Compose setup, the named volume `pulse_data` is mounted at `/data`, and `PULSE_DB` points to `/data/pulse.db` on that volume.

```yaml
volumes:
  - pulse_data:/data
```

To back up Pulse, stop the container and copy the SQLite file (and the data directory, if `PULSE_DATA_DIR` is set) from the volume.

## The SQLite file

Pulse uses a single SQLite file as its only data store. It holds the admin account, monitors and groups, check history, incidents and updates, maintenance windows, status pages, themes, API keys, and notification settings.

Raw check history is pruned by the worker according to the [data retention](monitors.md#data-retention) window, which defaults to 90 days.
