package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/memetics19/pulse/agent/internal/collector"
	"github.com/memetics19/pulse/agent/internal/diagnostics"
	"github.com/memetics19/pulse/agent/internal/pusher"
)

func main() {
	server := flag.String("server", "", "Pulse API base URL, e.g. https://status.example.com (required)")
	token := flag.String("token", "", "Bearer token for ingest authentication (required)")
	interval := flag.Int("interval", 30, "Push interval in seconds (default 30)")
	diagnose := flag.Bool("diagnose", false,
		"collect one diagnostic bundle, print it, and exit; also uploads it when --server and --token are set")
	flag.Parse()

	if *diagnose {
		runDiagnoseAndExit(*server, *token)
	}

	if *server == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "pulse-agent: --server and --token are required")
		flag.Usage()
		os.Exit(1)
	}
	if *interval < 1 {
		fmt.Fprintln(os.Stderr, "pulse-agent: --interval must be >= 1")
		os.Exit(1)
	}

	col := collector.New()
	psh := pusher.New(*server, *token)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("pulse-agent starting: server=%s interval=%ds", *server, *interval)

	// Push immediately on startup, then on each tick.
	if err := pushOnce(ctx, col, psh); err != nil {
		log.Printf("push error: %v", err)
	}

	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := pushOnce(ctx, col, psh); err != nil {
				log.Printf("push error: %v", err)
			}
		case <-ctx.Done():
			log.Println("pulse-agent shutting down")
			return
		}
	}
}

func pushOnce(ctx context.Context, col *collector.Collector, psh *pusher.Pusher) error {
	m, err := col.Snapshot()
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := psh.Push(ctx, m); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	log.Printf("pushed: cpu=%.1f%% mem=%.1f%% disk=%.1f%% net_rx=%d net_tx=%d",
		m.CpuPercent, m.MemPercent, m.DiskPercent, m.NetRxBytes, m.NetTxBytes)
	return nil
}

// diagnoseTimeout bounds a whole --diagnose run. Individual commands carry
// their own shorter timeout inside the ExecRunner.
const diagnoseTimeout = 60 * time.Second

// commandTimeout bounds each diagnostic command. A wedged host is exactly when
// diagnostics matter, so no single command may hang the run.
const commandTimeout = 10 * time.Second

// runDiagnoseAndExit performs a one-shot diagnostic collection and terminates.
// Uploading is optional: with no server configured the bundle still goes to
// stdout, which is the only mode available when Pulse itself is unreachable.
func runDiagnoseAndExit(server, token string) {
	if err := credentialError(server, token); err != nil {
		fmt.Fprintln(os.Stderr, "pulse-agent:", err)
		os.Exit(1)
	}

	var p diagnosticsPusher
	if server != "" && token != "" {
		p = pusher.New(server, token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), diagnoseTimeout)
	defer cancel()

	if err := runDiagnose(ctx, diagnostics.NewExecRunner(commandTimeout), p, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pulse-agent:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
