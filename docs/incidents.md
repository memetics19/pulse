# Incidents

An incident records a disruption to a service, its timeline of updates, and its resolution. Incidents appear on the [public status page](status-pages.md) for the monitors a page shows.

## Lifecycle

An incident moves through these states:

1. `detected`
2. `investigating`
3. `identified`
4. `implementing`
5. `monitoring`
6. `resolved`

## Required root cause analysis

A root cause analysis (RCA) is required before an incident can be resolved. You cannot move an incident to `resolved` until an RCA is recorded.

## Updates

Each incident has a timeline of updates. Updates support Markdown. There are two kinds of update:

- **Status change.** An update that advances the incident to a new state in the lifecycle.
- **Comment.** An update that adds information without changing the status.

## Auto-detection

The monitoring worker auto-creates an incident after two consecutive down checks for a monitor. The incident is named after the monitor. While a [maintenance window](maintenance.md) is active for a monitor, auto-incidents are suppressed for it.

## The public timeline

The public status page shows each incident with its full update timeline. Incident history is available on the page, and updates are also published through the [Atom feed](api.md#atom-feed). Times render in the visitor's local timezone.

## Notifications

When configured, Pulse sends incident notifications by email (through Resend) and Slack (through an incoming webhook). See [Configuration](configuration.md) for the variables that enable these channels.
