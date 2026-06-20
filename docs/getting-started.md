# Getting started

This page covers the requirements, the Docker Compose quick start, the first-run setup wizard, and reaching your public page.

## Requirements

- Docker with the Compose plugin.
- A persistent volume for the SQLite database.

Pulse runs as a single container. The image is a static binary on Alpine with ca-certificates. No external database server is required.

To serve a public page over HTTPS, you also need a reverse proxy in front of Pulse that holds TLS certificates and forwards to port 8080. See [Status pages](status-pages.md) for examples.

## Quick start with Docker Compose

Clone the repository and start the container:

```bash
git clone https://github.com/memetics19/pulse.git
cd pulse
docker compose up -d
```

The provided `docker-compose.yml` defines a single service and a named volume:

```yaml
services:
  pulse:
    image: pulse:latest
    ports:
      - "8080:8080"
    environment:
      SQLITE_PATH: /data/pulse.db
      RESEND_API_KEY: ${RESEND_API_KEY:-}
      SLACK_WEBHOOK_URL: ${SLACK_WEBHOOK_URL:-}
    volumes:
      - pulse_data:/data
    restart: unless-stopped

volumes:
  pulse_data:
```

Data persists on the `pulse_data` volume. The SQLite file lives at the path set by `SQLITE_PATH`, which defaults to `/data/pulse.db`. See [Configuration](configuration.md) for the full list of environment variables.

## First-run setup wizard

On first start, every route redirects to a setup wizard. The wizard collects:

1. **Admin account.** Name, email, username, and password. The password is hashed with Argon2id.
2. **Branding.** Logo and site name.
3. **SQLite path.** The location of the database file.

After you complete the wizard, the app runs normally and the setup routes no longer redirect.

Open `http://localhost:8080` to begin. If you are running behind a reverse proxy, open the host you configured.

## Reaching your public page

After setup, the default status page is available on the host that serves Pulse. To reach a public page such as `status.shreeda.xyz`, point a DNS record at your reverse proxy and configure the proxy to forward that host to Pulse on port 8080. Pulse resolves which page to show from the request Host header. See [Status pages](status-pages.md) for the full setup, including Caddy and nginx examples.

## Importing from Uptime Kuma

If you already track services in Uptime Kuma, the CLI imports your monitors and groups from an Uptime Kuma backup file. This is a one-time migration convenience.

### Export the backup file from Uptime Kuma

The import reads a backup file, not a live connection. Uptime Kuma does not provide a REST API for reading monitor configuration, and its API keys only grant access to the Prometheus metrics endpoint, which exposes monitor names and up or down state but not the URL, type, interval, or thresholds. The backup file is the only export that contains the full monitor configuration.

To produce it in Uptime Kuma:

1. Open Uptime Kuma and go to Settings.
2. Open the Backup section.
3. Choose Export and save the JSON file (for example `backup.json`).

### Run the import

```bash
pulse-cli import uptime-kuma \
  --file backup.json \
  --server https://status.example.com \
  --token pulse_live_xxxxxxxx
```

The command reads the backup file and creates the monitors in Pulse under a single "Imported" group, through the REST API. `--server` is your Pulse base URL and `--token` is an API key with the `monitors:write` scope (create one under Admin → API keys). Historical check data is not imported, because Uptime Kuma backups do not include it.

## Next steps

- Add [monitors](monitors.md) for the services you want to track.
- Configure [notifications](configuration.md) for email and Slack.
- Create additional [status pages](status-pages.md) for different audiences.
- Generate [API keys](api.md) for automation.
