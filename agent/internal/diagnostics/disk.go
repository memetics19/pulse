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

// alwaysFullFilesystems have no backing store and report 100% permanently, so
// flagging them would be a false "disk full" on a healthy host.
//
// tmpfs and overlay are deliberately absent: a full tmpfs is real memory-backed
// exhaustion, and a full overlay is a container's writable layer filling up.
// Both are actionable.
var alwaysFullFilesystems = map[string]bool{
	"devfs":    true,
	"devtmpfs": true,
	"udev":     true,
	"proc":     true,
	"sysfs":    true,
	"efivarfs": true,
}

// isAlwaysFull reports whether a mount reads 100% by design rather than being
// exhausted.
//
// Loop devices are deliberately not suppressed. A read-only image mount such as
// a snap is permanently 100%, but writable loop-mounted ext4/XFS is common in
// appliance and VM workflows, and df -P names the device rather than the
// filesystem type so the two are indistinguishable here. Hiding a genuinely
// full filesystem is worse than an occasional spurious flag.
func isAlwaysFull(filesystem string) bool {
	return alwaysFullFilesystems[filesystem]
}

// CollectDisk reports filesystem usage. POSIX output (-P) keeps each mount on
// a single line, which df does not otherwise guarantee for long device names.
// -k forces 1024-byte blocks: POSIX -P alone reports 512-byte blocks on BSD and
// under POSIXLY_CORRECT, which would make AvailableKb silently double.
func CollectDisk(ctx context.Context, r Runner) (DiskReport, error) {
	out, err := r.Run(ctx, "df", "-Pk")
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
		if capacity >= fullThresholdPercent && !isAlwaysFull(mount.Filesystem) {
			report.Full = append(report.Full, mount.Mount)
		}
	}
	return report
}
