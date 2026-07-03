# Pulse

Pulse is an open-source, self-hosted status page and monitoring tool. It ships as a single Go binary with a SQLite database. The monitoring worker runs in the same process, so there is no separate database server, Node.js runtime, or reverse proxy required to run it. Pulse checks your services, records uptime and latency, opens incidents when checks fail, and serves public status pages on your own domains.

A live status page runs at [status.shreeda.xyz](https://status.shreeda.xyz). The full documentation is at [docs.shreeda.xyz](https://docs.shreeda.xyz).

<!--![Pulse status page](docs/assets/status-page.gif)-->

## Highlights

- **Single binary and SQLite.** One Go binary plus one SQLite file. The monitoring worker runs in-process.
- **Multiple status pages on custom domains.** Each page has its own domain, title, and selected monitor groups. Pulse resolves the page from the request Host header.
- **Scoped API keys.** Named, revocable keys with granular scopes (for example `monitors:read`, `incidents:write`). Keys are shown once at creation and stored as a hash.
- **Built-in theming.** Presets, logo and favicon upload, a customizable footer, CSS variable overrides, per-page branding, and a visitor dark-mode toggle.
- **Structured incidents with required RCA.** A defined lifecycle from detected to resolved. A root cause analysis is required before an incident can be resolved.
- **Scheduled maintenance with alert suppression.** Maintenance windows transition automatically. While a window is active, alerts and auto-incidents are suppressed for the affected monitors.
- **TOTP two-factor authentication.** Optional time-based one-time passwords using an authenticator app.
- **Atom feed.** The public page exposes an Atom feed for incident updates.
- **Local-timezone rendering.** All times render in the visitor's local timezone.

## Quick start (60 seconds)

You need Docker with the Compose plugin.

The interactive installer asks a few questions (port, data directory, HTTPS, homelab LAN monitoring), then pulls the published image and starts Pulse:

```bash
curl -fsSL https://raw.githubusercontent.com/memetics19/pulse/main/deploy/install.sh | sh
```

Or run from a clone of the repository:

```bash
git clone https://github.com/memetics19/pulse.git
cd pulse
docker compose up -d
```

Open `http://localhost:8080`. On first start, every route redirects to a setup wizard. Create the admin account, set branding, and choose the SQLite path. After setup the app runs normally.

To serve a public page such as `status.shreeda.xyz` over HTTPS, run a reverse proxy in front of Pulse that terminates TLS and forwards to port 8080. See the status pages documentation for Caddy and nginx examples.

## Documentation

Full documentation is at [docs.shreeda.xyz](https://docs.shreeda.xyz). The Markdown sources live in [`docs/`](docs/index.md):

- [Getting started](docs/getting-started.md)
- [Configuration](docs/configuration.md)
- [Monitors](docs/monitors.md)
- [Incidents](docs/incidents.md)
- [Maintenance](docs/maintenance.md)
- [Status pages](docs/status-pages.md)
- [Theming](docs/theming.md)
- [API](docs/api.md)
- [Security](docs/security.md)
- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md)

## License

Pulse is released under the MIT License. See [LICENSE.md](LICENSE.md).
