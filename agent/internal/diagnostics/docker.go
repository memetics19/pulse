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

// DockerReport is the container section of a diagnostic bundle.
type DockerReport struct {
	Containers []Container `json:"containers"`
	NotRunning []string    `json:"not_running,omitempty"`
}

// CollectDocker lists all containers, including stopped ones — a container
// that is gone is exactly the thing being diagnosed.
func CollectDocker(ctx context.Context, r Runner) (DockerReport, error) {
	out, err := r.Run(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		return DockerReport{}, fmt.Errorf("docker ps: %w", err)
	}

	var report DockerReport
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c Container
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue // a malformed row must not cost us the rest
		}
		report.Containers = append(report.Containers, c)
		if c.State != "running" {
			report.NotRunning = append(report.NotRunning, c.Name)
		}
	}
	return report, nil
}
