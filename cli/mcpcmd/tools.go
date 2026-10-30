package mcpcmd

// tool maps one MCP tool to one Pulse REST route. Arguments named in the path
// as {name} fill the path, arguments listed in query go in the query string,
// and arguments listed in body are sent as the JSON request body.
type tool struct {
	name        string
	description string
	method      string
	path        string
	pathArgs    []string
	query       map[string]map[string]any
	body        map[string]map[string]any
	required    []string
	readOnly    bool
	destructive bool
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func ids(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": desc}
}
func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": desc}
}

var days = map[string]map[string]any{"days": integer("How many days back to read.")}

var monitorBody = map[string]map[string]any{
	"name":                  str("Display name."),
	"type":                  enum("Check type. push monitors wait for heartbeats instead of being checked.", "http", "https", "tcp", "ping", "dns", "ssl", "push"),
	"url":                   str("Target: a URL for http and https, host:port for tcp, a host name for ping, dns and ssl. Empty for push."),
	"interval_seconds":      integer("Seconds between checks."),
	"timeout_seconds":       integer("Seconds before a check fails. Default 30."),
	"expected_status":       integer("HTTP status the check expects. Leave out to accept any 2xx."),
	"keyword_check":         str("Text the HTTP response body must contain."),
	"degraded_threshold_ms": integer("Response time above which the monitor is degraded."),
	"down_threshold_ms":     integer("Response time above which the monitor is down. Must be above degraded_threshold_ms."),
	"is_active":             boolean("Whether the monitor is checked. Default true."),
	"group_id":              integer("Group the monitor belongs to."),
}

var groupBody = map[string]map[string]any{
	"name":          str("Group name."),
	"description":   str("Group description."),
	"display_order": integer("Position on the status page, lowest first."),
}

var notificationBody = map[string]map[string]any{
	"channel":     enum("Where alerts go.", "email", "slack"),
	"config_json": str(`Channel settings as a JSON string. email: {"to": "ops@example.com"}. slack: {"webhook_url": "https://hooks.slack.com/services/..."}.`),
	"monitor_id":  integer("Only alert for this monitor. Leave out to alert for every monitor."),
}

var pageBody = map[string]map[string]any{
	"domain":    str("Host name the page answers on, for example status.example.com."),
	"title":     str("Page title."),
	"published": boolean("Whether the page is public."),
	"group_ids": ids("Groups shown on the page, in order."),
}

var tools = []tool{
	// Read
	{name: "get_overview", description: "Summary of the whole instance: monitor counts by status, open incidents and active maintenance.", method: "GET", path: "/api/overview", readOnly: true},
	{name: "list_monitors", description: "List every monitor with its settings and current status.", method: "GET", path: "/api/monitors", readOnly: true},
	{name: "get_monitor", description: "Get one monitor.", method: "GET", path: "/api/monitors/{id}", pathArgs: []string{"id"}, readOnly: true},
	{name: "list_check_results", description: "Recent check results of a monitor: status, response time, status code and error. days defaults to 1, at most 90.", method: "GET", path: "/api/monitors/{monitor_id}/checks", pathArgs: []string{"monitor_id"}, query: days, readOnly: true},
	{name: "get_monitor_uptime", description: "Uptime percentage of a monitor. days defaults to 90, at most 90.", method: "GET", path: "/api/monitors/{monitor_id}/checks/uptime", pathArgs: []string{"monitor_id"}, query: days, readOnly: true},
	{name: "list_groups", description: "List monitor groups.", method: "GET", path: "/api/groups", readOnly: true},
	{name: "list_incidents", description: "List incidents, open and resolved.", method: "GET", path: "/api/incidents", readOnly: true},
	{name: "list_incident_updates", description: "The timeline updates of one incident.", method: "GET", path: "/api/incidents/{incident_id}/updates", pathArgs: []string{"incident_id"}, readOnly: true},
	{name: "list_maintenance", description: "List maintenance windows.", method: "GET", path: "/api/maintenance", readOnly: true},
	{name: "list_notifications", description: "List alert notifications and their channel settings.", method: "GET", path: "/api/notifications", readOnly: true},
	{name: "list_agents", description: "List infra agents and when each last reported.", method: "GET", path: "/api/agents", readOnly: true},
	{name: "get_agent_metrics", description: "Host metrics an agent reported: CPU, memory, disk and containers.", method: "GET", path: "/api/agents/{agent_id}/metrics", pathArgs: []string{"agent_id"}, query: days, readOnly: true},
	{name: "list_agent_diagnostics", description: "Diagnostics runs an agent reported.", method: "GET", path: "/api/agents/{agent_id}/diagnostics", pathArgs: []string{"agent_id"}, readOnly: true},
	{name: "list_pages", description: "List status pages and the groups each shows.", method: "GET", path: "/api/pages", readOnly: true},
	{name: "get_theme", description: "The status page theme.", method: "GET", path: "/api/theme", readOnly: true},

	// Monitors
	{name: "create_monitor", description: "Create a monitor.", method: "POST", path: "/api/monitors", body: monitorBody, required: []string{"name", "type"}},
	{name: "update_monitor", description: "Replace a monitor's settings. Send every field you want to keep: a field left out takes its default.", method: "PUT", path: "/api/monitors/{id}", pathArgs: []string{"id"}, body: monitorBody, required: []string{"name", "type"}},
	{name: "delete_monitor", description: "Delete a monitor and its check history.", method: "DELETE", path: "/api/monitors/{id}", pathArgs: []string{"id"}, destructive: true},
	{name: "rotate_push_token", description: "Give a push monitor a new heartbeat URL. The old URL stops working.", method: "POST", path: "/api/monitors/{id}/push-token/rotate", pathArgs: []string{"id"}, destructive: true},

	// Groups
	{name: "create_group", description: "Create a monitor group.", method: "POST", path: "/api/groups", body: groupBody, required: []string{"name"}},
	{name: "update_group", description: "Replace a group's name, description and order.", method: "PUT", path: "/api/groups/{id}", pathArgs: []string{"id"}, body: groupBody, required: []string{"name"}},
	{name: "delete_group", description: "Delete a group.", method: "DELETE", path: "/api/groups/{id}", pathArgs: []string{"id"}, destructive: true},

	// Incidents
	{name: "create_incident", description: "Open an incident. It shows on the status page and alerts the notification channels.", method: "POST", path: "/api/incidents", body: map[string]map[string]any{
		"title":                str("What is wrong, as visitors will read it."),
		"severity":             enum("How bad it is.", "minor", "major", "critical"),
		"affected_monitor_ids": ids("Monitors the incident affects."),
	}, required: []string{"title", "severity"}},
	{name: "update_incident_status", description: "Move an incident to a new status. Resolving needs an rca.", method: "PUT", path: "/api/incidents/{id}/status", pathArgs: []string{"id"}, body: map[string]map[string]any{
		"status": enum("New status.", "investigating", "identified", "implementing", "monitoring", "resolved"),
		"rca":    str("Root cause. Required when status is resolved."),
	}, required: []string{"status"}},
	{name: "post_incident_update", description: "Add an update to an incident's public timeline.", method: "POST", path: "/api/incidents/{incident_id}/updates", pathArgs: []string{"incident_id"}, body: map[string]map[string]any{
		"status":  enum("Status this update reports.", "investigating", "identified", "implementing", "monitoring", "resolved"),
		"message": str("What changed, as visitors will read it."),
		"author":  str("Who wrote the update."),
	}, required: []string{"status", "message"}},
	{name: "delete_incident", description: "Delete an incident and its timeline.", method: "DELETE", path: "/api/incidents/{id}", pathArgs: []string{"id"}, destructive: true},

	// Maintenance
	{name: "create_maintenance", description: "Schedule a maintenance window. Alerts and auto-incidents for its monitors are held while it runs.", method: "POST", path: "/api/maintenance", body: map[string]map[string]any{
		"title":                str("Title shown on the status page."),
		"description":          str("Details shown on the status page."),
		"affected_monitor_ids": ids("Monitors under maintenance."),
		"starts_at":            str("Start time, RFC 3339."),
		"ends_at":              str("End time, RFC 3339."),
		"start_now":            boolean("Start the window now instead of at starts_at."),
	}, required: []string{"title", "ends_at"}},
	{name: "update_maintenance_status", description: "Change a maintenance window's status, for example to cancel it.", method: "PUT", path: "/api/maintenance/{id}/status", pathArgs: []string{"id"}, body: map[string]map[string]any{
		"status": enum("New status.", "scheduled", "in_progress", "completed", "cancelled"),
	}, required: []string{"status"}},
	{name: "delete_maintenance", description: "Delete a maintenance window.", method: "DELETE", path: "/api/maintenance/{id}", pathArgs: []string{"id"}, destructive: true},

	// Notifications
	{name: "create_notification", description: "Add an alert channel.", method: "POST", path: "/api/notifications", body: notificationBody, required: []string{"channel", "config_json"}},
	{name: "update_notification", description: "Replace an alert channel's settings.", method: "PUT", path: "/api/notifications/{id}", pathArgs: []string{"id"}, body: notificationBody, required: []string{"channel", "config_json"}},
	{name: "delete_notification", description: "Delete an alert channel.", method: "DELETE", path: "/api/notifications/{id}", pathArgs: []string{"id"}, destructive: true},
	{name: "test_notification", description: "Send a test alert through one channel and report whether it was accepted.", method: "POST", path: "/api/notifications/{id}/test", pathArgs: []string{"id"}},

	// Agents
	{name: "create_agent", description: "Register an infra agent. The response holds its token once. Do not repeat it in chat unless asked.", method: "POST", path: "/api/agents", body: map[string]map[string]any{
		"name":       str("Unique agent name."),
		"host_label": str("Readable host name."),
	}, required: []string{"name"}},
	{name: "delete_agent", description: "Delete an agent. Its token stops working.", method: "DELETE", path: "/api/agents/{id}", pathArgs: []string{"id"}, destructive: true},

	// Status pages and theme
	{name: "create_page", description: "Create a status page on its own domain.", method: "POST", path: "/api/pages", body: pageBody, required: []string{"domain", "title"}},
	{name: "update_page", description: "Replace a status page's settings.", method: "PUT", path: "/api/pages/{id}", pathArgs: []string{"id"}, body: pageBody, required: []string{"domain", "title"}},
	{name: "delete_page", description: "Delete a status page.", method: "DELETE", path: "/api/pages/{id}", pathArgs: []string{"id"}, destructive: true},
	{name: "update_theme", description: "Replace the status page theme.", method: "PUT", path: "/api/theme", body: map[string]map[string]any{
		"preset":      enum("Built-in theme preset.", "default-light", "default-dark", "terminal-dark"),
		"custom_css":  str("Extra CSS for the status page."),
		"config_json": str("Theme settings as a JSON string."),
	}, required: []string{"preset"}},
}
