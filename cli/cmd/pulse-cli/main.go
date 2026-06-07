package main

import (
	"fmt"
	"os"

	"github.com/memetics19/pulse/cli/importcmd"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "pulse-cli",
		Short: "pulse-cli manages a Pulse status-page instance",
	}
	root.AddCommand(importcmd.NewImportCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
