package commands

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project and build status",
	Long:  `Show project metadata and the build state of each target.`,
	Args:  cobra.NoArgs,
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	fmt.Println(cmdHeader.Render("StarForge Project"))
	fmt.Println()
	fmt.Printf("  %-14s %s\n", "Name:", proj.Name)
	if proj.Description != "" {
		fmt.Printf("  %-14s %s\n", "Description:", proj.Description)
	}
	fmt.Printf("  %-14s %s\n", "Directory:", proj.Dir)
	fmt.Printf("  %-14s %s\n", "Build dir:", proj.BuildDir())
	fmt.Println()

	fmt.Println(cmdHeader.Render("Targets"))
	fmt.Println()

	names := make([]string, 0, len(proj.Targets))
	for name := range proj.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		target := proj.Targets[name]
		buildDir := proj.TargetBuildDir(name)

		state := statusNotBuilt.Render("[not built]")
		if _, err := os.Stat(buildDir); err == nil {
			state = statusBuilt.Render("[built]")
		}

		fmt.Printf("  %s %s\n", name, state)
		fmt.Printf("    Layers: %d\n", len(target.Layers))
		for _, layer := range target.Layers {
			fmt.Printf("      - %s\n", layer)
		}
	}

	return nil
}
