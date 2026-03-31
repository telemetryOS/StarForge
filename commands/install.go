package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/installer/client"
)

var installCmd = &cobra.Command{
	Use:    "install",
	Short:  "Run the StarForge installer TUI client",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		server, err := cmd.Flags().GetString("server")
		if err != nil {
			return fmt.Errorf("reading --server flag: %w", err)
		}
		unattended, err := cmd.Flags().GetBool("unattended")
		if err != nil {
			return fmt.Errorf("reading --unattended flag: %w", err)
		}

		if err := client.RunTUI(server, unattended); err != nil {
			return fmt.Errorf("installer TUI: %w", err)
		}
		return nil
	},
}

func init() {
	installCmd.Flags().String("server", "http://localhost:8100", "installer server URL")
	installCmd.Flags().Bool("unattended", false, "run in unattended mode")
}
