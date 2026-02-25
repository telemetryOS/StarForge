package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var cleanFlag bool

var buildCmd = &cobra.Command{
	Use:   "build [target]",
	Short: "Build overlay layers for a target",
	Long: `Resolve layers for a target and execute build phases using overlayfs caching.

Overlay layers are stored in the .starforge/ build directory. Unchanged phases
are skipped on subsequent builds. Partition images (.img files) are created on
demand by 'starforge run' or 'starforge write'.

Use --clean to force a full rebuild, ignoring the cache.`,
	Args: cobra.ExactArgs(1),
	RunE: runBuild,
}

func init() {
	buildCmd.Flags().BoolVar(&cleanFlag, "clean", false, "force a full rebuild, ignoring cache")
}

func runBuild(cmd *cobra.Command, args []string) error {
	targetName := args[0]

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	target, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	// Elevate to root early so cleanup, collect, and build all run privileged
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	builder := engine.NewBuilder(proj)
	if err := builder.Build(targetName, target, cleanFlag); err != nil {
		return err
	}

	// Ensure build artifacts are owned by the invoking user, not root
	engine.ChownToInvoker(proj.BuildDir())

	return nil
}
