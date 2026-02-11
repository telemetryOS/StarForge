package commands

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List targets and their layers",
	Long:  `Display the project name, description, and all defined targets with their constituent layers.`,
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	fmt.Println(proj.Name)
	if proj.Description != "" {
		fmt.Println(proj.Description)
	}
	fmt.Println()

	// Sort target names for deterministic output
	names := make([]string, 0, len(proj.Targets))
	for name := range proj.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		target := proj.Targets[name]
		fmt.Printf("  %s\n", name)
		for _, layer := range target.Layers {
			fmt.Printf("    - %s\n", layer)
		}
		fmt.Println()
	}

	return nil
}
