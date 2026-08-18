package diagnostics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Mount is one filesystem's usage as reported by df.
type Mount struct {
	Filesystem      string `json:"filesystem"`
	Mount           string `json:"mount"`
	CapacityPercent int    `json:"capacity_percent"`
	AvailableKb     int64  `json:"available_kb"`
}

// DiskReport is the filesystem section of a diagnostic bundle. Full lists the
// mounts at capacity, so "which disk filled up" is answerable without scanning
// every row — disk exhaustion is both a common failure and the trigger for the
// restart death spiral remediation has to avoid.
type DiskReport struct {
	Mounts []Mount  `json:"mounts"`
	Full   []string `json:"full,omitempty"`
}

// fullThresholdPercent is the capacity at which a mount is considered full.
// Below 100% there is usually still room for the reserved-blocks margin.
const fullThresholdPercent = 100

// pseudoFilesystems permanently report 100% capacity because they have no
// backing store. Flagging them as full would produce a false "disk full"
// diagnosis on a perfectly healthy host.
var pseudoFilesystems = map[string]bool{
	"devfs":    true,
	"devtmpfs": true,
	"tmpfs":    true,
	"udev":     true,
	"overlay":  true,
	"squashfs": true,
	"proc":     true,
	"sysfs":    true,
	"efivarfs": true,
}

// CollectDisk reports filesystem usage. POSIX output (-P) keeps each mount on
// a single line, which df does not otherwise guarantee for long device names.
func CollectDisk(ctx context.Context, r Runner) (DiskReport, error) {
	out, err := r.Run(ctx, "df", "-P")
	if err != nil {
		return DiskReport{}, fmt.Errorf("df: %w", err)
	}
	return parseDF(string(out)), nil
}

func parseDF(out string) DiskReport {
	var report DiskReport
	for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		capacity, err := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
		if err != nil {
			continue
		}
		available, _ := strconv.ParseInt(fields[3], 10, 64)
		mount := Mount{
			Filesystem:      fields[0],
			Mount:           fields[5],
			CapacityPercent: capacity,
			AvailableKb:     available,
		}
		report.Mounts = append(report.Mounts, mount)
		// Every mount stays listed for context; only real storage can be full.
		if capacity >= fullThresholdPercent && !pseudoFilesystems[mount.Filesystem] {
			report.Full = append(report.Full, mount.Mount)
		}
	}
	return report
}
