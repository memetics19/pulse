package diagnostics

import "time"

// Section is one collector's contribution to a Bundle. Exactly one of Data or
// Error is set: a collector that failed contributes its error, never partial
// data that could be mistaken for a reading.
type Section struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// Bundle is the diagnostic evidence collected from one host at one moment.
// It is stored server-side as a JSON payload and rendered on the incident.
type Bundle struct {
	CollectedAt time.Time          `json:"collected_at"`
	Sections    map[string]Section `json:"sections"`
}

// Add records a collector's result under name. A non-nil err degrades that
// section alone — hosts routinely deny dmesg, lack docker, or are not Proxmox
// nodes, and none of that should cost us the sections that did work.
func (b *Bundle) Add(name string, data any, err error) {
	if b.Sections == nil {
		b.Sections = make(map[string]Section)
	}
	if err != nil {
		b.Sections[name] = Section{Error: err.Error()}
		return
	}
	b.Sections[name] = Section{Data: data}
}
