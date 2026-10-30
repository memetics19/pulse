# Monitors

A monitor defines a check that Pulse runs on a schedule. Each result records whether the target is up, degraded, or down, along with the response latency. The monitoring worker runs in the same process as the rest of Pulse.

## Monitor types

| Type | What it checks |
| --- | --- |
| `http` | An HTTP or HTTPS endpoint. |
| `tcp` | A TCP connection to a host and port. |
| `ping` | ICMP reachability of a host. |
| `dns` | DNS resolution for a name. |
| `ssl` | A TLS certificate on a host and port. |
| `infra` | Host metrics pushed by the Pulse infra agent (CPU, memory, disk, network). |

## Per-monitor options

Each monitor has the following options. Not every option applies to every type.

| Option | Description |
| --- | --- |
| Interval | How often the check runs. See [First check after a restart](#first-check-after-a-restart). |
| Timeout | How long a single check waits before it is considered a failure. |
| Expected status code | For HTTP checks, the status code that counts as a success when set. |
| Keyword check | For HTTP checks, a string that must appear in the response body. See [HTTP options](#http-options) to require that it does not appear. |
| Degraded threshold | A latency above which a result is marked degraded instead of up. |
| Down threshold | A latency above which a result is marked down. |
| Group | The group the monitor belongs to. |

HTTP monitors have more options. See [HTTP options](#http-options).

## HTTP options

These options apply to `http` monitors only. Every option is optional. Leave them all out and the check works as before. Pulse ignores them for other monitor types. In the admin, they sit under HTTP options in the monitor form.

| Option | Default | Description |
| --- | --- | --- |
| `method` | `GET` | `GET`, `HEAD` or `POST`. |
| `headers` | none | Extra request headers, as a map of name to value. |
| `body` | empty | The request body. It is sent with `POST` only and is capped at 64 KiB. |
| `auth` | none | Basic auth or a bearer token. See [Auth](#auth). |
| `tls_verify` | `true` | Set it to `false` to skip certificate checks. |
| `ca_pem` | empty | A PEM CA certificate to trust, for a private or self-signed CA. |
| `follow_redirects` | `true` | Set it to `false` to check the first response, such as a `301`. |
| `accepted_statuses` | empty | Status codes and ranges that count as up, such as `["200-299", "301"]`. |
| `keyword_mode` | `contains` | `contains` or `not_contains`. With `not_contains` the check is down when the keyword appears. |

### Headers

A monitor can set up to 32 headers. Each value is capped at 4 KiB. Some headers are rejected because the HTTP client sets them or they control the connection: `Host`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Keep-Alive`, `Proxy-Connection`, `Upgrade`, `TE`, `Trailer`, `Expect` and `Proxy-Authorization`. Set `Authorization` with `auth` instead, so the secret stays write-only.

Header values are returned by the API like any other monitor field. Put a secret in `auth`, not in a header, when it should not be readable later.

A header can replace the default `User-Agent`. A `POST` body is sent without a `Content-Type` unless you add one.

### Auth

`auth` is either basic auth with a username and password, or a bearer token. The password and the token are write-only. The API never returns them. It returns the auth type, the basic username and a `secret_set` flag.

The secret is stored in the database as plain text. It is not encrypted at rest, so protect the database file the same way you protect the admin login.

### TLS

By default the check verifies the server certificate against the system roots. For a homelab service with a private CA, paste the CA certificate into `ca_pem`. Pulse then trusts that CA on top of the system roots. This is safer than turning verification off. Set `tls_verify` to `false` only when you cannot get the CA. With verification off, `ca_pem` is not used.

### Redirects

Redirects are followed by default, up to 10. Every hop goes through the same private network guard as the first request, so a redirect cannot reach an internal address unless `PULSE_ALLOW_PRIVATE_MONITORS` is on. When a redirect leaves the monitored host and port, Pulse drops the auth and the monitor's headers from the next request.

With `follow_redirects` set to `false`, the check judges the redirect response itself. Set an expected status or accepted statuses such as `301` or `302` to make it count as up.

### Status codes

With no expected status and no accepted statuses, any `2xx` is up. When either is set, the response must match the expected status or fall in one of the accepted statuses. Codes run from 100 to 599. A range such as `200-399` includes both ends.

## First check after a restart

When Pulse starts, it does not check every monitor at once. Each monitor waits a short offset before its first check. The offset is under 30 seconds and never longer than the monitor's interval. It comes from the monitor's ID, so a monitor keeps the same offset across restarts. Monitors with the same interval stay spread out instead of checking in the same second.

A monitor you create or edit while Pulse runs checks right away. Push monitors are not affected, since they wait for heartbeats.

## Groups

Monitors are organized into groups. A [status page](status-pages.md) selects which groups it displays, so groups are the unit you use to decide what a given page shows. The default page shows all groups.

## How an HTTP check decides up, degraded, or down

The HTTP check sends a User-Agent header with the request, plus any [HTTP options](#http-options) the monitor sets. Then:

- Any `2xx` response is treated as up, unless the monitor sets an expected status or accepted statuses.
- A transient failure (a network error or a `5xx` response) triggers one retry. If the retry also fails, the result is recorded as down.
- When a degraded or down latency threshold is set, a response slower than the threshold is marked degraded or down even if the status is otherwise a success.

When a keyword check is configured, the body must also contain the keyword to count as up. With `keyword_mode` set to `not_contains`, the body must not contain it. A keyword check needs a body, so it cannot be used with `HEAD`.

## Down detection and incidents

After two consecutive down checks for a monitor, the worker auto-creates an incident named after the monitor. See [Incidents](incidents.md) for the lifecycle and the public timeline. While a [maintenance window](maintenance.md) is active for a monitor, alerts and auto-incidents are suppressed.

## Data retention

A configurable retention window controls how long history is kept. The default is 90 days. The worker prunes anything older than the window.

The window covers three data classes: raw check history, agent [diagnostic bundles](diagnostics.md), and the 1-minute agent metrics behind the host charts. Lowering it discards all three, so choose it with the diagnostic evidence in mind and not only uptime storage.

Raw agent samples are rolled up into 1-minute averages and kept for 24 hours. Login sessions are deleted as soon as they expire.
