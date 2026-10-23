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
	"github.com/memetics19/pulse/agent/internal/diagnostics"
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
	// --token is gone. Reject it before flag parsing so the user gets the
	// replacement instead of a generic "flag provided but not defined".
	if usesRemovedTokenFlag(args) {
		_, _ = fmt.Fprintln(stderr, errTokenFlagRemoved)
		return 2
	}

	fs := flag.NewFlagSet("pulse-agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), usageHeader)
		fs.PrintDefaults()
	}
	server := fs.String("server", "", "Pulse API base URL, e.g. https://status.example.com (required)")
	tokenFile := fs.String("token-file", "",
		"File to read the bearer token from (takes precedence over PULSE_AGENT_TOKEN)")
	interval := fs.Int("interval", 30, "Push interval in seconds")
	diagnose := fs.Bool("diagnose", false,
		"collect one diagnostic bundle, print it, and exit; also uploads it when --server and a token are set")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	tok, err := resolveToken(*tokenFile)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "pulse-agent:", err)
		return 1
	}
	if *diagnose {
		// Local-only mode ignores a token from the environment: without --server
		// there is nowhere to send it.
		if *server == "" && *tokenFile == "" {
			tok = ""
		}
		return diagnoseOnce(ctx, *server, tok, fs.NArg(), os.Stdout, stderr)
	}
	if *server == "" || tok == "" {
		_, _ = fmt.Fprintln(stderr, "pulse-agent: --server and a token (--token-file or PULSE_AGENT_TOKEN) are required")
		return 1
	}
	if *interval < 1 {
		_, _ = fmt.Fprintln(stderr, "pulse-agent: --interval must be >= 1")
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
			// select picks randomly when the ticker and ctx.Done are both
			// ready, so check for cancellation first. Otherwise shutdown can
			// run several more ~500ms collections before it is noticed.
			if ctx.Err() != nil {
				log.Println("pulse-agent shutting down")
				return
			}
			if err := pushOnce(ctx, col, psh); err != nil {
				log.Printf("push error: %v", err)
			}
		case <-ctx.Done():
			log.Println("pulse-agent shutting down")
			return
		}
	}
}

// usageHeader leads the --help output. It names both token sources and their
// precedence, since neither shows up as an ordinary flag default.
const usageHeader = `Usage: pulse-agent --server URL [flags]

The bearer token comes from --token-file when it is set, and otherwise from the
PULSE_AGENT_TOKEN environment variable. There is no --token flag: a token on
the command line is visible to every local user through ps and /proc.

Flags:
`

// errTokenFlagRemoved is printed when someone still passes --token.
const errTokenFlagRemoved = "pulse-agent: --token was removed in 3.0 because it exposed the token in ps and /proc. " +
	"Set PULSE_AGENT_TOKEN or pass --token-file instead."

// usesRemovedTokenFlag reports whether args contain -token or --token, with or
// without "=value". It stops at "--", after which everything is positional.
// The scan is deliberately broader than the flag parser, which stops at the
// first positional argument: a stray --token anywhere is still a token on the
// command line, and the point is to say so.
func usesRemovedTokenFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		name, ok := strings.CutPrefix(a, "-")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(name, "-")
		name, _, _ = strings.Cut(name, "=")
		if name == "token" {
			return true
		}
	}
	return false
}

// resolveToken returns the bearer token. An explicit --token-file wins over
// the PULSE_AGENT_TOKEN environment variable, because a flag on this command
// line is a more specific instruction than an inherited environment. An empty
// token file is an error rather than a silent fall back to the environment.
func resolveToken(tokenFile string) (string, error) {
	if tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("reading --token-file: %w", err)
		}
		tok := strings.TrimSpace(string(b))
		if tok == "" {
			return "", fmt.Errorf("--token-file %s is empty", tokenFile)
		}
		return tok, nil
	}
	return strings.TrimSpace(os.Getenv("PULSE_AGENT_TOKEN")), nil
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

// commandTimeout bounds each diagnostic command. A wedged host is exactly when
// diagnostics matter, so no single command may hang the run.
const commandTimeout = 10 * time.Second

// diagnoseOnce performs a one-shot diagnostic collection and returns the exit
// code. Uploading is optional: with no server configured the bundle still goes
// to stdout, which is the only mode available when Pulse itself is unreachable.
func diagnoseOnce(ctx context.Context, server, token string, nargs int, stdout, stderr io.Writer) int {
	if err := validateDiagnoseFlags(server, token, nargs); err != nil {
		_, _ = fmt.Fprintln(stderr, "pulse-agent:", err)
		return 1
	}

	var p diagnosticsPusher
	if server != "" && token != "" {
		p = pusher.New(server, token)
	}

	// No overall deadline: every command is already bounded by commandTimeout,
	// and a shared budget only let one slow section starve the others. Ctrl-C
	// still aborts through ctx.
	if err := runDiagnose(ctx, diagnostics.NewExecRunner(commandTimeout), p, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "pulse-agent:", err)
		return 1
	}
	return 0
}
