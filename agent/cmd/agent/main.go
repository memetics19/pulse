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
	"github.com/memetics19/pulse/agent/internal/pusher"
)

func main() {
	server := flag.String("server", "", "Pulse API base URL, e.g. https://status.example.com (required)")
	token := flag.String("token", "", "Bearer token for ingest authentication (required)")
	interval := flag.Int("interval", 30, "Push interval in seconds (default 30)")
	flag.Parse()

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
