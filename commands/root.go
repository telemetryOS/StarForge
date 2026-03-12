package commands

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "starforge",
	Short: "Declarative Arch Linux OS image builder",
	Long:  "StarForge builds custom Arch Linux OS images from declarative layer-based recipes.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(chrootCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(writeCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(installServerCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(upgradeCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
