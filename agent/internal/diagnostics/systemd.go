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

// SystemdReport is the service-manager section of a diagnostic bundle.
type SystemdReport struct {
	FailedUnits []Unit `json:"failed_units"`
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
	return report, nil
}
