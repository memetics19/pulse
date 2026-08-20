package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Container is one entry from the Docker container list. Status carries the
// exit reason for stopped containers — "Exited (137)" is SIGKILL, which on a
// container almost always means the OOM killer.
type Container struct {
	ID     string `json:"ID"`
	Image  string `json:"Image"`
	Name   string `json:"Names"`
	State  string `json:"State"`
	Status string `json:"Status"`
}

// DockerReport is the container section of a diagnostic bundle. Logs holds the
// recent output of each container that is not running, keyed by name.
type DockerReport struct {
	Containers []Container       `json:"containers"`
	NotRunning []string          `json:"not_running,omitempty"`
	Logs       map[string]string `json:"logs,omitempty"`
}

// CollectDocker lists all containers, including stopped ones — a container
// that is gone is exactly the thing being diagnosed.
func CollectDocker(ctx context.Context, r Runner) (DockerReport, error) {
	out, err := r.Run(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		return DockerReport{}, fmt.Errorf("docker ps: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))

	var report DockerReport
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c Container
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue // one malformed row must not cost us the rest
		}
		report.Containers = append(report.Containers, c)
		if c.State != "running" {
			report.NotRunning = append(report.NotRunning, c.Name)
		}
	}

	// Output that yields nothing is not a healthy empty section: docker prints
	// nothing at all when there are no containers, so text we could not parse
	// means the section is unreliable and should say so.
	if trimmed != "" && len(report.Containers) == 0 {
		return DockerReport{}, fmt.Errorf("docker ps: no containers parsed from %d bytes of output", len(trimmed))
	}

	report.Logs = collectContainerLogs(ctx, r, report.NotRunning)
	return report, nil
}

// collectContainerLogs fetches recent output for containers that are not
// running. A container whose logs cannot be read is skipped rather than
// failing the section.
func collectContainerLogs(ctx context.Context, r Runner, names []string) map[string]string {
	var logs map[string]string
	for _, name := range capTargets(names) {
		out, err := r.Run(ctx, "docker", "logs", "--tail", logTailLines, name)
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
