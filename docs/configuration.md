# Configuration

Pulse is configured with environment variables. Notification channels are optional and stay disabled when their variables are unset.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `SQLITE_PATH` | `/data/pulse.db` | Path to the SQLite database file. |
| `PULSE_DATA_DIR` | (unset) | Directory for Pulse data such as uploaded assets. |
| `API_PORT` | `8080` | TCP port the HTTP server listens on. |
| `RESEND_API_KEY` | (unset) | API key for sending email notifications through Resend. Leave unset to disable email. |
| `SLACK_WEBHOOK_URL` | (unset) | Slack incoming webhook URL for notifications. Leave unset to disable Slack. |
| `PULSE_SECURE_COOKIES` | `false` | Set to `true` when Pulse is served over HTTPS to mark session cookies `Secure`. |

Set these in the `environment` block of `docker-compose.yml` or in your container runtime.

```yaml
environment:
  SQLITE_PATH: /data/pulse.db
  API_PORT: "8080"
  RESEND_API_KEY: ${RESEND_API_KEY:-}
  SLACK_WEBHOOK_URL: ${SLACK_WEBHOOK_URL:-}
```

## Data persistence

Pulse stores all state in the SQLite database file. Mount a persistent volume so the file survives container restarts and upgrades. In the default Compose setup, the named volume `pulse_data` is mounted at `/data`, and `SQLITE_PATH` points to `/data/pulse.db` on that volume.

```yaml
volumes:
  - pulse_data:/data
```

To back up Pulse, stop the container and copy the SQLite file (and the data directory, if `PULSE_DATA_DIR` is set) from the volume.

## The SQLite file

Pulse uses a single SQLite file as its only data store. It holds the admin account, monitors and groups, check history, incidents and updates, maintenance windows, status pages, themes, API keys, and notification settings.

Raw check history is pruned by the worker according to the [data retention](monitors.md#data-retention) window, which defaults to 90 days.
