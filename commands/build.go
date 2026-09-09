package commands

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/executor"
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

	proj, target, err := loadProjectAndTarget(targetName)
	if err != nil {
		return err
	}

	buildExecutor, err := selectedBuildExecutor()
	if err != nil {
		return err
	}
	if err := buildExecutor.Prepare(cmd.Context()); err != nil {
		return err
	}

	buildDir := proj.TargetBuildDir(targetName)
	os.MkdirAll(buildDir, 0o755)

	output, err := engine.InitOutput(buildDir, "build", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	return output.Run(func() error {
		_, err := buildExecutor.Build(cmd.Context(), executor.BuildRequest{
			Project:    proj,
			TargetName: targetName,
			Target:     target,
			Clean:      cleanFlag,
		})
		return err
	})
}
