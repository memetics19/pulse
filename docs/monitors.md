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
| Interval | How often the check runs. |
| Timeout | How long a single check waits before it is considered a failure. |
| Expected status code | For HTTP checks, the status code that counts as a success when set. |
| Keyword check | For HTTP checks, a string that must appear in the response body. |
| Degraded threshold | A latency above which a result is marked degraded instead of up. |
| Down threshold | A latency above which a result is marked down. |
| Group | The group the monitor belongs to. |

## Groups

Monitors are organized into groups. A [status page](status-pages.md) selects which groups it displays, so groups are the unit you use to decide what a given page shows. The default page shows all groups.

## How an HTTP check decides up, degraded, or down

The HTTP check sends a User-Agent header with the request. Then:

- Any `2xx` response is treated as up.
- A transient failure (a network error or a `5xx` response) triggers one retry. If the retry also fails, the result is recorded as down.
- When a degraded or down latency threshold is set, a response slower than the threshold is marked degraded or down even if the status is otherwise a success.

When an expected status code or keyword check is configured, the response must also match that condition to count as up.

## Down detection and incidents

After two consecutive down checks for a monitor, the worker auto-creates an incident named after the monitor. See [Incidents](incidents.md) for the lifecycle and the public timeline. While a [maintenance window](maintenance.md) is active for a monitor, alerts and auto-incidents are suppressed.

## Data retention

A configurable retention window controls how long raw check history is kept. The default is 90 days. The worker prunes results older than the window.
