package commands

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/executor"
	"github.com/telemetryos/starforge/machine"
)

var runSerial bool

var runCmd = &cobra.Command{
	Use:   "run <target>",
	Short: "Boot a target in QEMU",
	Long: `Boot the current sealed full-disk artifact in a sibling QEMU virtual
machine for testing, building an artifact first when none exists. Run
"starforge build" after changing project inputs. QEMU uses a disposable
snapshot, so the build artifact is never modified by the running target.

SSH is forwarded on port 2222: ssh -p 2222 localhost`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

func init() {
	runCmd.Flags().BoolVar(&runSerial, "serial", false, "attach serial console to terminal")
}

func runRun(cmd *cobra.Command, args []string) error {
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

	output, err := engine.InitOutput(buildDir, "run", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	var artifactResult *executor.ArtifactResult
	err = output.Run(func() error {
		var buildErr error
		artifactResult, buildErr = buildExecutor.EnsureArtifact(cmd.Context(), executor.BuildRequest{
			Project:    proj,
			TargetName: targetName,
			Target:     target,
		})
		return buildErr
	})
	if err != nil {
		return err
	}

	runner := &machine.QEMU{}
	return runner.Run(cmd.Context(), machine.RunRequest{
		Artifact: artifactResult.Receipt,
		Disk:     artifactResult.Disk,
		BuildDir: buildDir,
		Config:   target.QEMU,
		Serial:   runSerial,
	})
}
