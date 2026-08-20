package diagnostics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// VM is one guest as reported by the Proxmox host.
type VM struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ProxmoxReport is the hypervisor section of a diagnostic bundle. It is
// collected from the host rather than inside a guest: a hung VM cannot report
// on itself, so the host-side view is the only reliable one.
type ProxmoxReport struct {
	VMs        []VM     `json:"vms"`
	NotRunning []string `json:"not_running,omitempty"`
}

// CollectProxmox lists guests via qm. On a non-Proxmox host qm is absent and
// the section degrades to an error, which is correct rather than fatal.
func CollectProxmox(ctx context.Context, r Runner) (ProxmoxReport, error) {
	out, err := r.Run(ctx, "qm", "list")
	if err != nil {
		return ProxmoxReport{}, fmt.Errorf("qm list: %w", err)
	}

	var report ProxmoxReport
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		vmid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		vm := VM{VMID: vmid, Name: fields[1], Status: fields[2]}
		report.VMs = append(report.VMs, vm)
		if vm.Status != "running" {
			report.NotRunning = append(report.NotRunning, vm.Name)
		}
	}
	return report, nil
}
