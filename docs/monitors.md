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
| Keyword check | For HTTP checks, a string that must appear in the response body. |
| Degraded threshold | A latency above which a result is marked degraded instead of up. |
| Down threshold | A latency above which a result is marked down. |
| Failures before incident | How many down checks in a row open an incident. See [Sensitivity](#sensitivity). |
| Recoveries before resolve | How many up or degraded checks in a row resolve that incident. |
| Retries | For HTTP, TCP and TLS checks, how many more attempts one check makes after a transient failure. |
| Retry interval | How long a check waits before each retry. |
| Certificate warning days | For SSL checks, the days left at which the certificate is marked degraded. The default is 14. |
| Certificate critical days | For SSL checks, the days left at which the certificate is marked down. The default is 7. |
| Group | The group the monitor belongs to. |

## Sensitivity

Four settings decide how quickly Pulse reacts to a failing monitor. Each monitor has its own values.

| Setting | API field | Default | Range |
| --- | --- | --- | --- |
| Failures before incident | `failures_before_incident` | 2 | 1 to 10 |
| Recoveries before resolve | `recoveries_before_resolve` | 2 | 1 to 10 |
| Retries | `retries` | 1 | 0 to 5 |
| Retry interval | `retry_interval_ms` | 500 | 0 to 60000 |

Retries happen inside one check. A check that fails and then passes on a retry is recorded as one up result. A check that fails on every attempt is recorded as one down result. So retries smooth out single blips, and failures before incident decides how many failed checks it takes to open an incident.

An example. A monitor checks every 30 seconds with the defaults. A failing check is retried once after 500 ms. The second failed check in a row opens an incident, within about a minute of the outage. Set failures before incident to 1 for a critical service, so the first failed check opens an incident. Set it to 5 for a flaky upstream, so an incident needs about two and a half minutes of failures.

Only transient failures are retried:

- HTTP retries a network error or a `5xx` response. A `4xx` response or a keyword mismatch is not retried.
- TCP retries any failed connection.
- TLS retries a failed connection or handshake. An expired or expiring certificate is not retried.

Ping and DNS checks do not retry. Push monitors ignore the retry settings but use the failure and recovery counts.

Each retry can take up to the monitor's timeout. Keep the timeout times the number of attempts below the interval, or checks run back to back.

Changing any of these settings restarts the monitor, like any other edit. The next check uses the new values.

## First check after a restart

When Pulse starts, it does not check every monitor at once. Each monitor waits a short offset before its first check. The offset is under 30 seconds and never longer than the monitor's interval. It comes from the monitor's ID, so a monitor keeps the same offset across restarts. Monitors with the same interval stay spread out instead of checking in the same second.

A monitor you create or edit while Pulse runs checks right away. Push monitors are not affected, since they wait for heartbeats.

## Groups

Monitors are organized into groups. A [status page](status-pages.md) selects which groups it displays, so groups are the unit you use to decide what a given page shows. The default page shows all groups.

## How an HTTP check decides up, degraded, or down

The HTTP check sends a User-Agent header with the request. Then:

- Any `2xx` response is treated as up.
- A transient failure (a network error or a `5xx` response) triggers a retry. By default there is one retry after 500 ms. If every retry also fails, the result is recorded as down. See [Sensitivity](#sensitivity).
- When a degraded or down latency threshold is set, a response slower than the threshold is marked degraded or down even if the status is otherwise a success.

When an expected status code or keyword check is configured, the response must also match that condition to count as up.

## How an SSL check decides up, degraded, or down

The SSL check opens a TLS connection to the host and port and reads the certificate the server sends. Then:

- A failed connection, a failed handshake, or a certificate that does not verify is down. An expired certificate is down.
- The days left are counted in whole days, rounded down. A certificate with 30 hours left has 1 day left.
- With the warning days or fewer left, the result is degraded.
- With the critical days or fewer left, the result is down.
- Otherwise the result is up.

The warning days must be greater than the critical days, and the critical days must be 0 or more. The API rejects other values. The defaults are 14 and 7. A team that renews 90-day certificates at 30 days left might set 29 and 14. Set the critical days to 0 to stay degraded until the last day. On that day less than a full day is left, so the check is down.

Each SSL result stores the days left in `cert_days_left`. The admin monitor list shows them as "expires in N days" next to the monitor type.

For one release, the days left are also written to `status_code`, as earlier releases did. Read `cert_days_left` instead. A later release stops writing the days left to `status_code`. The upgrade copies the days left of older SSL results into `cert_days_left`.

A change to either threshold applies from the next check, without a restart.

## Down detection and incidents

After the monitor's failures before incident count of down checks in a row (2 by default), the worker auto-creates an incident named after the monitor. After its recoveries before resolve count of up or degraded checks in a row (2 by default), the worker resolves that incident. See [Incidents](incidents.md) for the lifecycle and the public timeline. While a [maintenance window](maintenance.md) is active for a monitor, alerts and auto-incidents are suppressed.

## Data retention

A configurable retention window controls how long history is kept. The default is 90 days. The worker prunes anything older than the window.

The window covers three data classes: raw check history, agent [diagnostic bundles](diagnostics.md), and the 1-minute agent metrics behind the host charts. Lowering it discards all three, so choose it with the diagnostic evidence in mind and not only uptime storage.

Raw agent samples are rolled up into 1-minute averages and kept for 24 hours. Login sessions are deleted as soon as they expire.
