package uptimekuma

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ukMonitor mirrors the relevant fields in an Uptime Kuma backup monitorList entry.
type ukMonitor struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Type          string   `json:"type"`
	Interval      int64    `json:"interval"`
	Maxretries    int64    `json:"maxretries"`
	Keyword       string   `json:"keyword"`
	InvertKeyword flexBool `json:"invertKeyword"`

	// HTTP options.
	Method              string          `json:"method"`
	Body                string          `json:"body"`
	Headers             json.RawMessage `json:"headers"`
	AuthMethod          string          `json:"authMethod"`
	BasicAuthUser       string          `json:"basic_auth_user"`
	BasicAuthPass       string          `json:"basic_auth_pass"`
	IgnoreTLS           flexBool        `json:"ignoreTls"`
	MaxRedirects        *int64          `json:"maxredirects"`
	AcceptedStatusCodes []string        `json:"accepted_statuscodes"`
}

// flexBool decodes a JSON boolean that older Uptime Kuma versions may write
// as 0 or 1.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch string(bytes.TrimSpace(data)) {
	case "true", "1":
		*b = true
	default:
		*b = false
	}
	return nil
}

// ukBackup is the top-level structure of an Uptime Kuma JSON backup.
type ukBackup struct {
	MonitorList []ukMonitor `json:"monitorList"`
}

// PulseMonitor is the Pulse API payload for POST /api/monitors (minus group_id,
// which the importer sets after creating the group).
type PulseMonitor struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	Type            string `json:"type"`
	IntervalSeconds int64  `json:"interval_seconds"`
	KeywordCheck    string `json:"keyword_check"`

	// HTTP options. Empty values keep the Pulse defaults.
	Method           string            `json:"method,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Body             string            `json:"body,omitempty"`
	Auth             *PulseAuth        `json:"auth,omitempty"`
	TLSVerify        *bool             `json:"tls_verify,omitempty"`
	FollowRedirects  *bool             `json:"follow_redirects,omitempty"`
	AcceptedStatuses []string          `json:"accepted_statuses,omitempty"`
	KeywordMode      string            `json:"keyword_mode,omitempty"`
}

// PulseAuth is the auth object of a Pulse HTTP monitor.
type PulseAuth struct {
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

// allowedTypes is the set of Pulse monitor types that are accepted after mapping.
var allowedTypes = map[string]bool{
	"http": true,
	"tcp":  true,
	"ping": true,
	"dns":  true,
	"ssl":  true,
}

// mapType converts an Uptime Kuma type string to the equivalent Pulse type.
// "push" and "keyword" → "http". All other values pass through unchanged.
func mapType(ukType string) string {
	if ukType == "push" || ukType == "keyword" {
		return "http"
	}
	return ukType
}

// Parse decodes an Uptime Kuma JSON backup and returns:
//   - monitors: slice of PulseMonitor ready to POST to Pulse
//   - skipped: names of monitors whose type or HTTP options Pulse does not support
//   - err: non-nil if the JSON is malformed
func Parse(data []byte) (monitors []PulseMonitor, skipped []string, err error) {
	var backup ukBackup
	if err = json.Unmarshal(data, &backup); err != nil {
		return nil, nil, err
	}

	for _, m := range backup.MonitorList {
		pulseType := mapType(m.Type)
		if !allowedTypes[pulseType] {
			skipped = append(skipped, m.Name)
			continue
		}
		pm := PulseMonitor{
			Name:            m.Name,
			URL:             m.URL,
			Type:            pulseType,
			IntervalSeconds: m.Interval,
			KeywordCheck:    m.Keyword,
		}
		if pulseType == "http" && !mapHTTPOptions(m, &pm) {
			skipped = append(skipped, m.Name)
			continue
		}
		monitors = append(monitors, pm)
	}
	return monitors, skipped, nil
}

// mapHTTPOptions copies the Uptime Kuma HTTP options Pulse supports. It
// returns false when the monitor needs an option Pulse does not have, such as
// a PUT request or NTLM auth, so it is skipped instead of imported broken.
func mapHTTPOptions(m ukMonitor, pm *PulseMonitor) bool {
	method := strings.ToUpper(strings.TrimSpace(m.Method))
	switch method {
	case "", "GET", "HEAD":
	case "POST":
		pm.Body = m.Body
	default:
		return false
	}
	if method != "GET" {
		pm.Method = method
	}

	headers, ok := parseHeaders(m.Headers)
	if !ok {
		return false
	}
	// Pulse keeps auth secrets write-only, so an Authorization header becomes
	// bearer auth. Any other Authorization scheme cannot be carried over.
	for name, value := range headers {
		if !strings.EqualFold(name, "Authorization") {
			continue
		}
		token, isBearer := strings.CutPrefix(value, "Bearer ")
		if !isBearer || token == "" {
			return false
		}
		pm.Auth = &PulseAuth{Type: "bearer", Token: token}
		delete(headers, name)
	}
	if len(headers) > 0 {
		pm.Headers = headers
	}

	switch m.AuthMethod {
	case "", "basic":
		// Older backups set basic_auth_user without an authMethod.
		if m.BasicAuthUser != "" {
			if pm.Auth != nil {
				return false
			}
			pm.Auth = &PulseAuth{Type: "basic", Username: m.BasicAuthUser, Password: m.BasicAuthPass}
		}
	default:
		return false
	}

	if m.IgnoreTLS {
		off := false
		pm.TLSVerify = &off
	}
	if m.MaxRedirects != nil && *m.MaxRedirects == 0 {
		off := false
		pm.FollowRedirects = &off
	}
	pm.AcceptedStatuses = m.AcceptedStatusCodes
	if m.InvertKeyword && m.Keyword != "" {
		pm.KeywordMode = "not_contains"
	}
	return true
}

// parseHeaders reads Uptime Kuma's headers field. Backups store it as a JSON
// string holding a JSON object, and it may be null or empty.
func parseHeaders(raw json.RawMessage) (map[string]string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, true
		}
		raw = json.RawMessage(text)
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, false
	}
	return headers, true
}
