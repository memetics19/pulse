# API

Pulse exposes a REST API for automation. Requests authenticate with a scoped API key sent as a Bearer token.

## Base path

The API is served by the same process as the rest of Pulse, on the port set by `API_PORT` (default `8080`). API routes are under `/api`. The examples below assume Pulse is reachable at `https://status.shreeda.xyz`.

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

## Atom feed

The public status page exposes an Atom feed of incident updates at `/feed.xml`. A Subscribe control on the page links to it.

```bash
curl https://status.shreeda.xyz/feed.xml
```

The feed is public and does not require an API key.
