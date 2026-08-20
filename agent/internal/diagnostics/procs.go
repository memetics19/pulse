package diagnostics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Process is one entry from the process table, ordered by CPU share.
type Process struct {
	PID        int     `json:"pid"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	Command    string  `json:"command"`
}

// ProcessReport is the process-table section of a diagnostic bundle. It answers
// "what was eating the box" — a runaway transcode shows up here as ffmpeg long
// before the service it belongs to reports anything wrong.
type ProcessReport struct {
	Top []Process `json:"top"`
}

// topProcessCount bounds the section size; the tail is noise for diagnosis.
const topProcessCount = 15

// CollectProcesses reports the busiest processes, highest CPU first.
func CollectProcesses(ctx context.Context, r Runner) (ProcessReport, error) {
	out, err := r.Run(ctx, "ps", "-eo", "pid,pcpu,pmem,comm", "--sort=-pcpu")
	if err != nil {
		return ProcessReport{}, fmt.Errorf("ps: %w", err)
	}
	return parsePS(string(out)), nil
}

func parsePS(out string) ProcessReport {
	var report ProcessReport
	for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		mem, _ := strconv.ParseFloat(fields[2], 64)
		report.Top = append(report.Top, Process{
			PID:        pid,
			CPUPercent: cpu,
			MemPercent: mem,
			Command:    strings.Join(fields[3:], " "),
		})
		if len(report.Top) == topProcessCount {
			break
		}
	}
	return report
}
