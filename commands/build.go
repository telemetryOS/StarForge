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
	Short: "Build and seal a target OS image",
	Long: `Resolve a target, execute its build phases, and seal a full-disk artifact.

With --executor=auto, ordinary users build inside a managed QEMU appliance;
root on a matching architecture uses the native backend. Build caches and
artifacts are scoped by target architecture, machine, and distribution.

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
