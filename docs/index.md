# Pulse documentation

Pulse is an open-source, self-hosted status page and monitoring tool. It runs as a single Go binary with a SQLite database. The monitoring worker runs in the same process. There is no separate database server, Node.js runtime, or reverse proxy required to run it.

Pulse checks your services on a schedule, records uptime and latency, opens incidents when checks fail, and serves public status pages on your own domains. A live example runs at [status.shreeda.xyz](https://status.shreeda.xyz). These docs are hosted at [docs.shreeda.xyz](https://docs.shreeda.xyz).

![Pulse status page](assets/status-page.gif)

## Highlights

- **Single binary and SQLite.** One Go binary plus one SQLite file. The monitoring worker runs in-process.
- **Multiple status pages on custom domains.** Each page has its own domain, title, and selected monitor groups. Pulse resolves the page from the request Host header.
- **Scoped API keys.** Named, revocable keys with granular scopes. Keys are shown once at creation and stored as a hash.
- **Built-in theming.** Presets, logo and favicon upload, a customizable footer, CSS variable overrides, per-page branding, and a visitor dark-mode toggle.
- **Structured incidents with required RCA.** A defined lifecycle from detected to resolved, with a required root cause analysis before resolution.
- **Scheduled maintenance with alert suppression.** Windows transition automatically and suppress alerts for affected monitors while active.
- **TOTP two-factor authentication.** Optional time-based one-time passwords using an authenticator app.
- **Atom feed.** The public page exposes an Atom feed for incident updates.
- **Local-timezone rendering.** All times render in the visitor's local timezone.

## Table of contents

| Page | Topic |
| --- | --- |
| [Getting started](getting-started.md) | Requirements, the Docker Compose quick start, and the first-run setup wizard. |
| [Configuration](configuration.md) | Environment variables, data persistence, and the SQLite file. |
| [Monitors](monitors.md) | The six monitor types, per-monitor options, groups, and how a check decides up, degraded, or down. |
| [Incidents](incidents.md) | The incident lifecycle, required RCA, Markdown updates, auto-detection, and the public timeline. |
| [Maintenance](maintenance.md) | Scheduling, automatic transitions, alert suppression, and public display. |
| [Status pages](status-pages.md) | The default page, creating pages, custom domains with reverse-proxy TLS, group selection, and scoping. |
| [Theming](theming.md) | Presets, logo and favicon, footer fields, CSS variable overrides, per-page branding, dark mode, and timezones. |
| [API](api.md) | The REST API base path, API keys and scopes, Bearer usage, curl examples, and the Atom feed. |
| [Security](security.md) | Password hashing, sessions, TOTP, password reset, and API key handling. |
| [Architecture](architecture.md) | The single binary, the in-process worker, the embedded admin, SQLite, and host-based page resolution. |
| [Deployment](deployment.md) | The GHCR image, release-please versioning, and the CI/CD release-and-deploy flow to the homelab. |
| [Roadmap](roadmap.md) | Planned features that are not yet built. |
