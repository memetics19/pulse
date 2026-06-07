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
git clone https://github.com/your-org/pulse.git
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

If you already track services in Uptime Kuma, the CLI can import monitors and groups from an Uptime Kuma backup file. This is a migration convenience for bringing existing checks into Pulse.

```bash
pulse-cli import uptime-kuma --file backup.json
```

The command reads the backup file and creates the corresponding monitors and groups in Pulse.

## Next steps

- Add [monitors](monitors.md) for the services you want to track.
- Configure [notifications](configuration.md) for email and Slack.
- Create additional [status pages](status-pages.md) for different audiences.
- Generate [API keys](api.md) for automation.
