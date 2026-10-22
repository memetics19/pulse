# Roadmap

What has shipped, what is being finished now, and what comes next. Items under Next and
Later are not built yet, so do not rely on them. The other documentation pages describe
only what exists today.

## Shipped

- **1.x.** Single Go binary with SQLite and an in-process monitoring worker. HTTP, TCP,
  TLS certificate, ping and DNS monitors, and host metrics pushed by the infra agent.
  Multiple status pages on custom domains, incidents with a required root cause,
  maintenance windows with alert suppression, scoped API keys, theming, TOTP, an Atom feed,
  and email (Resend) and Slack alerts. Since 1.1.0, release binaries for Linux and macOS on
  amd64 and arm64, with an installer that picks the right one and checks it.
- **Since 1.1.0, on main.** Import from Uptime Kuma, Next.js 16 for the admin UI with lint
  in CI, and CI for all three Go modules.

## Now

Work that is open or merging for the next release.

- **Security and reliability hardening.** The SSRF guard covers every monitor type, the
  public `/api/status` is scoped to its status page and returns only what the page shows,
  theme CSS and links are sanitized, SQLite really runs in WAL mode, the worker restarts
  with backoff and `/healthz` reports a stalled worker, and each Go module has a coverage
  gate in CI. The `/api/status` change is breaking, so this release is 2.0.0.
- **Agent host diagnostics.** The agent collects a diagnostic bundle from the host it runs
  on and Pulse stores it. Each release also publishes the agent and CLI binaries.
- **Supply chain.** Trivy scans every pull request, `main` and a weekly run, on the
  repository and on both images, and fails on fixable HIGH or CRITICAL findings. Go 1.27,
  patched dependencies, current Alpine, and images that run as a non-root user.
- **Self-updating deploy.** status.shreeda.xyz and docs.shreeda.xyz update themselves from
  each release and from `main`. The host pulls, so GitHub holds no SSH key, password or VPN
  key for it.

## Next

- **Signed releases.** Cosign signatures and an SBOM for the images and binaries, and
  provenance attestations, so a download can be checked against this repository.
- **Multi-arch images.** `linux/arm64` images next to `linux/amd64`, and the agent image
  published to GHCR too.
- **Dependency updates for everything.** Dependabot for Go modules, npm, GitHub Actions and
  Docker base images, grouped weekly. Actions pinned to a commit.
- **Upgrade tests.** CI migrates a database from the previous release to the current schema,
  so an upgrade never meets a migration for the first time in production.
- **More alert channels.** ntfy, Gotify, Discord, Telegram and generic webhooks, next to
  email and Slack.
- **Maintenance notifications** and **recurring maintenance windows.**

## Later

- **PostgreSQL backend** as an option next to SQLite.
- **Automatic TLS for custom domains.** Today a reverse proxy in front of Pulse terminates
  TLS. See [Status pages](status-pages.md).
- **Subscribers.** Visitors subscribe to email or Slack updates from the public page. Today
  the page offers an Atom feed.
- **History downsampling.** Older check history kept as lower-resolution summaries instead
  of being pruned after the retention window.
- **Several admin users with roles**, and single sign-on (OIDC) for the admin UI.
- **Prometheus metrics** for monitors and the worker.
