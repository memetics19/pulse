# Roadmap

The items below are planned and not yet built. They are listed here for transparency. Do not rely on them today. The documentation pages describe only what currently exists.

## Planned

- **PostgreSQL backend.** SQLite is the only data store today. A PostgreSQL option is planned.
- **Automatic TLS for custom domains.** Today you terminate TLS at a reverse proxy in front of Pulse. Built-in certificate management is planned. See [Status pages](status-pages.md) for the current reverse-proxy setup.
- **Public email and Slack subscriber management.** The public page offers an Atom feed today. Letting visitors subscribe to email or Slack updates from the public page is planned.
- **Tiered history downsampling.** Today raw check history is pruned after the retention window. Downsampling older history into lower-resolution summaries is planned.
- **Maintenance notifications.** Sending notifications for maintenance windows is planned.
- **Recurring maintenance.** Repeating maintenance schedules are planned. Today each window is created individually.
- **Multiple admin users and roles.** Today there is a single admin account. Multiple users with roles are planned.
