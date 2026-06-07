package uptimekuma

import "encoding/json"

// ukMonitor mirrors the relevant fields in an Uptime Kuma backup monitorList entry.
type ukMonitor struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Type       string `json:"type"`
	Interval   int64  `json:"interval"`
	Maxretries int64  `json:"maxretries"`
	Keyword    string `json:"keyword"`
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
// "push" → "http". All other values pass through unchanged.
func mapType(ukType string) string {
	if ukType == "push" {
		return "http"
	}
	return ukType
}

// Parse decodes an Uptime Kuma JSON backup and returns:
//   - monitors: slice of PulseMonitor ready to POST to Pulse
//   - skipped: names of monitors whose type was not supported after mapping
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
		monitors = append(monitors, PulseMonitor{
			Name:            m.Name,
			URL:             m.URL,
			Type:            pulseType,
			IntervalSeconds: m.Interval,
			KeywordCheck:    m.Keyword,
		})
	}
	return monitors, skipped, nil
}
