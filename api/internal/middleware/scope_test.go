package middleware

import (
	"testing"
)

func TestRequiredScope(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"GET", "/api/monitors", "monitors:read"},
		{"POST", "/api/monitors", "monitors:write"},
		{"GET", "/api/groups/1", "monitors:read"},
		{"GET", "/api/incidents", "incidents:read"},
		{"PUT", "/api/incidents/1/status", "incidents:write"},
		{"GET", "/api/notifications", "notifications:read"},
		{"POST", "/api/agents", "agents:write"},
		{"GET", "/api/theme", "theme:read"},
		{"POST", "/api/pages", "pages:write"},
		{"DELETE", "/api/maintenance/1", "maintenance:write"},
		{"GET", "/api/overview", "status:read"},
		{"GET", "/api/keys", ""},     // session-only
		{"GET", "/api/whatever", ""}, // unknown
	}
	for _, c := range cases {
		if got := requiredScope(c.method, c.path); got != c.want {
			t.Errorf("requiredScope(%s,%s)=%q want %q", c.method, c.path, got, c.want)
		}
	}
}
