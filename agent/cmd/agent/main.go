package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/memetics19/pulse/agent/internal/collector"
	"github.com/memetics19/pulse/agent/internal/pusher"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(parseAndRun(ctx, os.Args[1:], os.Stderr))
}

// parseAndRun parses flags, resolves the token, validates, and runs the agent.
// It returns a process exit code so the flag/validation branches are testable
// without os.Exit. run blocks until ctx is cancelled.
func parseAndRun(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("pulse-agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", "", "Pulse API base URL, e.g. https://status.example.com (required)")
	token := fs.String("token", "", "Bearer token (INSECURE: visible in ps/proc; prefer PULSE_AGENT_TOKEN or --token-file)")
	tokenFile := fs.String("token-file", "", "File to read the bearer token from")
	interval := fs.Int("interval", 30, "Push interval in seconds (default 30)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	tok, err := resolveToken(*token, *tokenFile)
	if err != nil {
		fmt.Fprintln(stderr, "pulse-agent:", err)
		return 1
	}
	if *server == "" || tok == "" {
		fmt.Fprintln(stderr, "pulse-agent: --server and a token (PULSE_AGENT_TOKEN, --token-file, or --token) are required")
		return 1
	}
	if *interval < 1 {
		fmt.Fprintln(stderr, "pulse-agent: --interval must be >= 1")
		return 1
	}

	run(ctx, *server, tok, time.Duration(*interval)*time.Second)
	return 0
}

// run pushes a metrics snapshot immediately, then on every interval, until ctx
// is cancelled.
func run(ctx context.Context, serverURL, token string, interval time.Duration) {
	col := collector.New()
	psh := pusher.New(serverURL, token)

	log.Printf("pulse-agent starting: server=%s interval=%s", serverURL, interval)

	if err := pushOnce(ctx, col, psh); err != nil {
		log.Printf("push error: %v", err)
	}

	ticker := time.NewTicker(interval)
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

// resolveToken picks the bearer token from, in order of preference:
// PULSE_AGENT_TOKEN env var, --token-file contents, then --token. The env var
// and file are preferred because a --token flag is visible to any local user
// via ps(1) and /proc/<pid>/cmdline for the agent's whole lifetime.
func resolveToken(flagToken, tokenFile string) (string, error) {
	if env := os.Getenv("PULSE_AGENT_TOKEN"); env != "" {
		return strings.TrimSpace(env), nil
	}
	if tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("reading --token-file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(flagToken), nil
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
