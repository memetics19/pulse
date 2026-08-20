package diagnostics

import (
	"context"
	"fmt"
	"strings"
)

// Unit is one systemd unit in the failed state.
type Unit struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

// SystemdReport is the service-manager section of a diagnostic bundle. Logs
// holds the recent journal for each failed unit, keyed by unit name.
type SystemdReport struct {
	FailedUnits []Unit            `json:"failed_units"`
	Logs        map[string]string `json:"logs,omitempty"`
}

// CollectSystemd lists units in the failed state. --plain and --no-legend strip
// the status glyphs and summary footer, leaving one unit per line.
func CollectSystemd(ctx context.Context, r Runner) (SystemdReport, error) {
	out, err := r.Run(ctx, "systemctl", "list-units", "--failed", "--no-legend", "--plain")
	if err != nil {
		return SystemdReport{}, fmt.Errorf("systemctl list-units: %w", err)
	}

	var report SystemdReport
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		report.FailedUnits = append(report.FailedUnits, Unit{
			Unit:        fields[0],
			Load:        fields[1],
			Active:      fields[2],
			Sub:         fields[3],
			Description: strings.Join(fields[4:], " "),
		})
	}

	report.Logs = collectUnitJournals(ctx, r, report.FailedUnits)
	return report, nil
}

// collectUnitJournals fetches the recent journal for each failed unit. A unit
// whose journal cannot be read is skipped rather than failing the section.
func collectUnitJournals(ctx context.Context, r Runner, units []Unit) map[string]string {
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.Unit)
	}

	var logs map[string]string
	for _, name := range capTargets(names) {
		out, err := r.Run(ctx, "journalctl", "-u", name, "-n", logTailLines, "--no-pager")
		if err != nil {
			continue
		}
		if logs == nil {
			logs = make(map[string]string)
		}
		logs[name] = truncateLog(out)
	}
	return logs
}
