package commands

import (
	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/upgrade"
)

var upgradeCmd = &cobra.Command{
	Use:               "upgrade",
	Short:             "Upgrade StarForge to the latest version",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(_ *cobra.Command, _ []string) error {
		return upgrade.Upgrade()
	},
}
