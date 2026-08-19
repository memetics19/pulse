package diagnostics

import (
	"regexp"
	"strconv"
)

// OOMKill records one out-of-memory kill found in the kernel ring buffer.
// These explain a large share of services that die with no application-level
// error, so they are the first thing an RCA should look for.
type OOMKill struct {
	Process   string `json:"process"`
	PID       int    `json:"pid"`
	AnonRSSKb int64  `json:"anon_rss_kb"`
}

// oomKillRe matches the kernel's summary line, which is stable across kernel
// versions and carries the process, pid, and resident size in one place:
//
//	Out of memory: Killed process 1234 (jellyfin) total-vm:...kB, anon-rss:7104928kB, ...
//	Memory cgroup out of memory: Killed process 4821 (ffmpeg) total-vm:...kB, ...
//
// The case-insensitive match covers both the global form and the memory-cgroup
// form, which is what a container killed by its own memory limit logs — the
// primary Docker failure mode.
var oomKillRe = regexp.MustCompile(
	`(?i)out of memory: Killed process (\d+) \(([^)]+)\)(?:.*?anon-rss:(\d+)kB)?`)

// parseOOMKills returns every OOM kill recorded in dmesg output, oldest first.
// Output with no OOM kills yields an empty slice.
func parseOOMKills(dmesg string) []OOMKill {
	matches := oomKillRe.FindAllStringSubmatch(dmesg, -1)
	kills := make([]OOMKill, 0, len(matches))
	for _, m := range matches {
		pid, _ := strconv.Atoi(m[1])
		var anonRSS int64
		if m[3] != "" {
			anonRSS, _ = strconv.ParseInt(m[3], 10, 64)
		}
		kills = append(kills, OOMKill{Process: m[2], PID: pid, AnonRSSKb: anonRSS})
	}
	return kills
}
