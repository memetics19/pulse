package importcmd

import (
	"fmt"
	"os"

	"github.com/memetics19/pulse/cli/internal/pulseclient"
	"github.com/memetics19/pulse/cli/internal/uptimekuma"
	"github.com/spf13/cobra"
)

// NewImportCmd returns the "import" parent command.
func NewImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import resources into Pulse from external sources",
	}
	cmd.AddCommand(newUptimeKumaCmd())
	return cmd
}

func newUptimeKumaCmd() *cobra.Command {
	var file, server, token string

	cmd := &cobra.Command{
		Use:   "uptime-kuma",
		Short: "Import monitors from an Uptime Kuma JSON backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(file, server, token)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to Uptime Kuma backup JSON (required)")
	cmd.Flags().StringVarP(&server, "server", "s", "", "Pulse API base URL, e.g. http://localhost:8080 (required)")
	cmd.Flags().StringVarP(&token, "token", "t", "", "Pulse API key (required)")
	cobra.CheckErr(cmd.MarkFlagRequired("file"))
	cobra.CheckErr(cmd.MarkFlagRequired("server"))
	cobra.CheckErr(cmd.MarkFlagRequired("token"))
	return cmd
}

// runImport orchestrates the full import: parse → create group → create monitors → print summary.
func runImport(file, server, token string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading backup file: %w", err)
	}

	monitors, skipped, err := uptimekuma.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing backup: %w", err)
	}

	client := pulseclient.New(server, token)

	groupID, err := client.CreateGroup("Imported")
	if err != nil {
		return fmt.Errorf("creating group: %w", err)
	}

	var importedCount int
	for _, m := range monitors {
		if err := client.CreateMonitor(m, groupID); err != nil {
			return fmt.Errorf("creating monitor %q: %w", m.Name, err)
		}
		importedCount++
	}

	fmt.Printf("Import complete: %d monitors imported, %d skipped\n", importedCount, len(skipped))
	if len(skipped) > 0 {
		fmt.Println("Skipped monitors (unsupported type):")
		for _, name := range skipped {
			fmt.Printf("  - %s\n", name)
		}
	}
	return nil
}
