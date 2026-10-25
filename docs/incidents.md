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

When configured, Pulse sends incident notifications by email (through an SMTP relay or Resend) and Slack (through an incoming webhook). See [Configuration](configuration.md) for the variables that enable these channels.

Each notification has a channel and a small JSON config, checked when it is saved:

| Channel | Config | Rules |
| --- | --- | --- |
| `email` | `{"to": "ops@example.com"}` | One plain address. |
| `slack` | `{"webhook_url": "https://hooks.slack.com/services/..."}` | `https` only, no credentials in the URL. Leave it out to use `SLACK_WEBHOOK_URL`. |

Other keys are refused. A webhook may not point at a private or internal address (the
LAN, loopback, link-local or cloud metadata addresses) unless
`PULSE_ALLOW_PRIVATE_MONITORS=true`, the same rule monitors follow. The address is checked
when the notification is saved, again before every send, and once more when the connection
is opened. Redirects are not followed.

A notification quotes text that the admin did not write: a push monitor's `msg`, the error
a remote server returned, and monitor names set through the API. Pulse escapes all of it.
An email has an HTML part, with every field escaped, and a plain text part. A Slack message
escapes `&`, `<` and `>`, so no field can add a link or a mention such as `@channel`.
