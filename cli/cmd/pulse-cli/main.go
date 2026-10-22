package main

import (
	"fmt"
	"os"

	"github.com/memetics19/pulse/cli/importcmd"
	"github.com/spf13/cobra"
)

func main() { os.Exit(run(os.Args[1:])) }

// run builds the root command, executes it with args, and returns a process
// exit code (so it is testable without os.Exit).
func run(args []string) int {
	root := &cobra.Command{
		Use:   "pulse-cli",
		Short: "pulse-cli manages a Pulse status-page instance",
	}
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs(args)
	root.AddCommand(importcmd.NewImportCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
