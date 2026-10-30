# MCP server

`pulse-cli mcp` serves the Pulse API as [Model Context Protocol](https://modelcontextprotocol.io) tools. An AI assistant such as Claude can then read your monitors and incidents, open and resolve incidents, schedule maintenance, and manage monitors, groups, notifications, agents and status pages.

It runs on your machine and talks to the assistant over stdin and stdout. It calls the Pulse REST API with an API key, so it needs nothing new on the server.

## Set it up

1. [Install pulse-cli](getting-started.md#install-pulse-cli).
2. In the admin, open **API keys** and create a key. Give it only the scopes the assistant needs. A key with only read scopes gives a read-only assistant.
3. Add the server to your MCP client. For Claude Code:

```bash
claude mcp add pulse \
  --env PULSE_URL=https://status.example.com \
  --env PULSE_API_KEY=pulse_live_... \
  -- pulse-cli mcp
```

For a client configured with JSON, such as Claude Desktop:

```json
{
  "mcpServers": {
    "pulse": {
      "command": "pulse-cli",
      "args": ["mcp"],
      "env": {
        "PULSE_URL": "https://status.example.com",
        "PULSE_API_KEY": "pulse_live_..."
      }
    }
  }
}
```

## Settings

| Setting | Meaning |
| --- | --- |
| `PULSE_URL` or `--server` | Base URL of your Pulse instance. |
| `PULSE_API_KEY` | The API key. |
| `--token-file` | A file that holds the API key. It is read only when `PULSE_API_KEY` is unset. |

The key is never taken from a flag, so it does not show up in process listings.

## Tools

Each tool calls one API route. The key's scopes decide which tools work. A read tool needs the area's `:read` scope and a write tool its `:write` scope. A tool whose scope the key lacks returns the API's 403 to the assistant.

| Area | Tools | Scope |
| --- | --- | --- |
| Overview | `get_overview` | `status:read` |
| Monitors | `list_monitors`, `get_monitor`, `list_check_results`, `get_monitor_uptime`, `create_monitor`, `update_monitor`, `delete_monitor`, `rotate_push_token` | `monitors:read`, `monitors:write` |
| Groups | `list_groups`, `create_group`, `update_group`, `delete_group` | `monitors:read`, `monitors:write` |
| Incidents | `list_incidents`, `list_incident_updates`, `create_incident`, `update_incident_status`, `post_incident_update`, `delete_incident` | `incidents:read`, `incidents:write` |
| Maintenance | `list_maintenance`, `create_maintenance`, `update_maintenance_status`, `delete_maintenance` | `maintenance:read`, `maintenance:write` |
| Notifications | `list_notifications`, `create_notification`, `update_notification`, `delete_notification`, `test_notification` | `notifications:read`, `notifications:write` |
| Agents | `list_agents`, `get_agent_metrics`, `list_agent_diagnostics`, `create_agent`, `delete_agent` | `agents:read`, `agents:write`, and `diagnostics:read` for diagnostics |
| Status pages | `list_pages`, `create_page`, `update_page`, `delete_page`, `get_theme`, `update_theme` | `pages:read`, `pages:write`, `theme:read`, `theme:write` |

Read tools are marked read-only. Delete tools, and `rotate_push_token`, are marked destructive, so a client that asks before risky actions asks before these.

API keys cannot manage API keys, settings or two-factor login. Those stay in the admin.

## Errors

A failed call comes back to the assistant as a tool error with the API's status and message, for example `HTTP 409 from POST /api/agents: an agent named "web" already exists`. Arguments are checked before any request is sent: a missing required field, an ID that is not a positive integer, or an unknown argument never reaches the API. A response larger than 4 MiB is cut off and marked as truncated.
