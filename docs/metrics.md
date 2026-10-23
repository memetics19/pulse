# Metrics

Pulse can serve its monitor state in the Prometheus text format at `GET /metrics`. Use it to graph checks in Grafana or to alert from Prometheus or Alertmanager.

## Turn it on

The endpoint is off by default. While it is off, `/metrics` returns `404`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `PULSE_METRICS` | `false` | Set to `true` to serve `/metrics`. |
| `PULSE_METRICS_TOKEN` | (unset) | When set, scrapes must send `Authorization: Bearer <token>`. Other requests get `401`. |

```yaml
environment:
  PULSE_METRICS: "true"
  PULSE_METRICS_TOKEN: ${PULSE_METRICS_TOKEN:-}
```

Set a token whenever Pulse is reachable from outside a trusted network. Serve Pulse over HTTPS so the token is not sent in clear text.

## Scrape config

```yaml
scrape_configs:
  - job_name: pulse
    scheme: https
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/pulse-token
    static_configs:
      - targets: ["status.example.com"]
```

Drop the `authorization` block when no token is set.

## Metrics

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `pulse_monitor_up` | gauge | `monitor`, `type`, `group` | Latest check of each active monitor. `1` is up, `0.5` is degraded, `0` is down. |
| `pulse_monitor_response_seconds` | gauge | `monitor`, `type`, `group` | Response time of the latest check, in seconds. |
| `pulse_monitor_checks_total` | counter | `monitor`, `result` | Check results recorded since Pulse started. `result` is `up`, `degraded` or `down`. |
| `pulse_incidents_open` | gauge | none | Incidents that are not resolved. |
| `pulse_worker_last_beat_timestamp_seconds` | gauge | none | Unix time of the worker's last successful run. It is `0` before the first run. `/healthz` uses the same heartbeat. |

The standard Go runtime (`go_*`) and process (`process_*`) metrics are served too.

Some notes on the values:

- Only active monitors are listed. A paused monitor has no series.
- A monitor with no checks yet has no `pulse_monitor_up` series. A check without a response time has no `pulse_monitor_response_seconds` series.
- `group` is empty for a monitor outside any group.
- `pulse_monitor_checks_total` starts from zero when Pulse restarts. Use `rate()` or `increase()` on it, as with any counter.
- Labels hold monitor names, types and groups only. Monitor targets such as URLs and hosts are never exposed.
- Series are keyed by name, type and group. Give monitors unique names. When two monitors share all three, only the older one is reported.

## Example alerts

```yaml
groups:
  - name: pulse
    rules:
      - alert: PulseMonitorDown
        expr: pulse_monitor_up == 0
        for: 5m
      - alert: PulseWorkerStalled
        expr: time() - pulse_worker_last_beat_timestamp_seconds > 120
        for: 2m
```
