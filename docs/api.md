# API

Pulse exposes a REST API for automation. Requests authenticate with a scoped API key sent as a Bearer token.

## Base path

The API is served by the same process as the rest of Pulse, on the port set by `PULSE_PORT` (default `8080`). API routes are under `/api`. The examples below assume Pulse is reachable at `https://status.shreeda.xyz`.

## API keys and scopes

Create API keys in the admin under API Keys. Each key is named and revocable. The full key is shown once at creation and only a hash is stored, so save the key when it is shown. A key carries one or more scopes:

| Scope | Grants |
| --- | --- |
| `monitors:read` | Read monitors. |
| `monitors:write` | Create and modify monitors. |
| `incidents:read` | Read incidents. |
| `incidents:write` | Create and modify incidents. |
| `notifications:read` | Read notification settings. |
| `notifications:write` | Modify notification settings. |
| `agents:read` | Read infra agents. |
| `agents:write` | Manage infra agents. |
| `diagnostics:read` | Read agent [diagnostic bundles](diagnostics.md). Separate from `agents:read` because bundles carry logs, process names, and paths. |
| `theme:read` | Read theme settings. |
| `theme:write` | Modify theme settings. |
| `maintenance:read` | Read maintenance windows. |
| `maintenance:write` | Create and modify maintenance windows. |
| `pages:read` | Read status pages. |
| `pages:write` | Create and modify status pages. |
| `status:read` | Read overall status data. |

## Bearer usage

Send the key in the `Authorization` header. Keys are prefixed with `pulse_live_`.

```
Authorization: Bearer pulse_live_...
```

## Examples

List monitors. This requires the `monitors:read` scope.

```bash
curl https://status.shreeda.xyz/api/monitors \
  -H "Authorization: Bearer pulse_live_..."
```

Create an incident. This requires the `incidents:write` scope.

```bash
curl -X POST https://status.shreeda.xyz/api/incidents \
  -H "Authorization: Bearer pulse_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Elevated error rates",
    "status": "investigating",
    "body": "We are investigating elevated error rates on the API."
  }'
```

Read the recent check results of a monitor. This requires the `monitors:read` scope. `days` defaults to 1 and is capped at 90.

```bash
curl "https://status.shreeda.xyz/api/monitors/12/checks?days=7" \
  -H "Authorization: Bearer pulse_live_..."
```

The same scope covers `/api/monitors/{id}/checks/uptime`, which returns the uptime percentage over `days` (default 90, capped at 90). Read the updates of an incident with `/api/incidents/{id}/updates` and the `incidents:read` scope. A request without a session or key gets 401. A key without the matching scope gets 403.

## Public endpoints

Only the routes below answer without a session or an API key. Everything else under `/api` returns 401 to an anonymous caller.

| Route | Access | Returns |
| --- | --- | --- |
| `GET /` | Public | The status page for the request Host. Monitor names and status, uptime bars, uptime percentage, average latency, incidents with their updates, and maintenance. |
| `GET /api/status` | Public | JSON for the same page. The overall status, each group with its monitor names and status, unresolved incidents (title, status, severity), and the theme preset. |
| `GET /feed.xml` | Public | Atom feed of the incidents on the same page. See [Atom feed](#atom-feed). |
| `GET /badge/status.svg`, `GET /badge/uptime.svg` | Public | SVG badges for the same page. See [Status badges](#status-badges). |
| `GET /static/*` | Public | Stylesheet, script and icon of the status page. |
| `GET /healthz` | Public | `{"status":"ok"}`, or 503 with `worker_unhealthy` when the worker has stopped. |
| `GET /metrics` | Bearer token when `PULSE_METRICS_TOKEN` is set | Prometheus metrics. Off unless `PULSE_METRICS=true`. See [Metrics](metrics.md). |
| `GET /api/auth/status` | Public | Whether setup is needed, and the caller's own login state. |
| `POST /api/auth/login`, `POST /api/auth/logout` | Public | Starts or ends a session. |
| `GET /api/setup/state`, `POST /api/setup` | Public | The first-run wizard. `POST` works once, before setup completes. |
| `GET` or `POST /api/push/{token}` | The push token in the URL | Records a heartbeat for a push monitor. Returns `{"ok":true}`. |
| `POST /api/ingest/metrics`, `POST /api/ingest/diagnostics` | Agent token | Accepts data from an infra agent. |
| `/admin/*`, `/setup/*`, `/_next/*`, `/favicon.ico` | Public | Static files of the admin app. The app loads its data through the authenticated API. |

The public status data is scoped to the page that matches the request Host. A page shows only the active monitors in its groups, and the incidents that touch them. Unpublished pages return 404. Public responses never carry a monitor target (URL, host or port), a raw check error, an HTTP status code, or the author of an incident update. A monitor shows only as up, degraded, down, or no data yet. The JSON spells the last one `unknown`.

## Status badges

Two public routes return a flat SVG badge for the page that matches the request Host. They follow the same scoping as `/api/status`.

| Route | Badge |
| --- | --- |
| `GET /badge/status.svg` | Overall status of the page: operational, degraded or outage. |
| `GET /badge/status.svg?monitor=<name or id>` | Status of one monitor: up, degraded, down or no data. |
| `GET /badge/uptime.svg` | Mean uptime of the page's monitors that have checks in the range. |
| `GET /badge/uptime.svg?monitor=<name or id>` | Uptime of one monitor. |

- `monitor` matches a monitor name, ignoring case, or a monitor ID. It only finds monitors the page shows. Any other value returns 404.
- `range` sets the uptime window: `24h`, `7d`, `30d` or `90d`. The default is `30d`. Any other value returns 400.
- An unpublished page returns 404.
- A badge shows only a monitor name, a status and an uptime percentage. It never shows a target or a raw error.
- Responses send `Cache-Control: public, max-age=60`.

```bash
curl 'https://status.example.com/badge/uptime.svg?monitor=API&range=7d'
```

## Atom feed

The public status page exposes an Atom feed of incident updates at `/feed.xml`. A Subscribe control on the page links to it.

```bash
curl https://status.shreeda.xyz/feed.xml
```

The feed is public and does not require an API key.
