package commands

import (
	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/executor"
)

var executorFlag string

var rootCmd = &cobra.Command{
	Use:           "starforge",
	Short:         "Declarative Arch Linux OS image builder",
	Long:          "StarForge builds custom Arch Linux OS images from declarative layer-based recipes.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&executorFlag, "executor", string(executor.ModeAuto), "build executor: auto, native, or vm")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(buildCmd)
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

func selectedBuildExecutor() (executor.BuildExecutor, error) {
	mode, err := executor.ParseMode(executorFlag)
	if err != nil {
		return nil, err
	}
	return executor.New(mode)
}

func Execute() error {
	return rootCmd.Execute()
}
