// Package diagnostics gathers read-only evidence about a host so an operator
// can tell why something broke without opening an SSH session.
//
// Every collector is read-only and takes a Runner, so collectors can be tested
// without depending on the machine the tests happen to run on.
package diagnostics

import (
	"context"
	"fmt"
	"time"
)

// Runner executes a diagnostic command and returns its output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// KernelReport is the kernel ring buffer section of a diagnostic bundle.
type KernelReport struct {
	OOMKills []OOMKill `json:"oom_kills"`
}

// CollectKernel reads the kernel ring buffer and extracts OOM kills.
func CollectKernel(ctx context.Context, r Runner) (KernelReport, error) {
	out, err := r.Run(ctx, "dmesg", "--ctime")
	if err != nil {
		return KernelReport{}, fmt.Errorf("dmesg: %w", err)
	}
	return KernelReport{OOMKills: parseOOMKills(string(out))}, nil
}

// Collect runs every collector and assembles a Bundle. It never returns an
// error: a host that denies dmesg or is not a Proxmox node still yields a
// useful bundle, with the unavailable sections carrying their own error.
func Collect(ctx context.Context, r Runner) Bundle {
	bundle := Bundle{CollectedAt: time.Now().UTC()}

	kernel, err := CollectKernel(ctx, r)
	bundle.Add("kernel", kernel, err)

	disk, err := CollectDisk(ctx, r)
	bundle.Add("disk", disk, err)

	procs, err := CollectProcesses(ctx, r)
	bundle.Add("processes", procs, err)

	docker, err := CollectDocker(ctx, r)
	bundle.Add("docker", docker, err)

	systemd, err := CollectSystemd(ctx, r)
	bundle.Add("systemd", systemd, err)

	proxmox, err := CollectProxmox(ctx, r)
	bundle.Add("proxmox", proxmox, err)

	return bundle
}
