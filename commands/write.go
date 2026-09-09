package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/executor"
)

var writeCmd = &cobra.Command{
	Use:   "write <target> <output>",
	Short: "Write a sealed target artifact to a block device",
	Long: `Build and verify a sealed full-disk target artifact, then write it to a
block device. Building is unprivileged when the VM executor is selected. Only
the final block-device operation is elevated.

The target device is repartitioned from the sealed artifact's recorded layout.
Growable partitions are expanded to fill available space. Use "starforge
export" to create regular disk image files.`,
	Args: cobra.ExactArgs(2),
	RunE: runWrite,
}

var writeArtifactCmd = &cobra.Command{
	Use:    "__write-artifact <target> <device>",
	Hidden: true,
	Args:   cobra.ExactArgs(2),
	RunE:   runWriteArtifact,
}

func runWrite(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	device, err := resolveBlockDevice(args[1])
	if err != nil {
		return err
	}
	project, target, err := loadProjectAndTarget(targetName)
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
	buildDir := project.TargetBuildDir(targetName)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	output, err := engine.InitOutput(buildDir, "write", targetName)
	if err != nil {
		return err
	}
	var artifactResult *executor.ArtifactResult
	if err := output.Run(func() error {
		var buildErr error
		artifactResult, buildErr = buildExecutor.EnsureArtifact(cmd.Context(), executor.BuildRequest{
			Project: project, TargetName: targetName, Target: target,
		})
		return buildErr
	}); err != nil {
		output.Close()
		return err
	}
	if err := artifact.NewStore(buildDir, targetName, target.ResolvedPlatform()).Verify(artifactResult.Receipt); err != nil {
		output.Close()
		return fmt.Errorf("verifying sealed artifact: %w", err)
	}
	output.Close()

	fmt.Printf("WARNING: All data on %s will be destroyed.\n", device)
	fmt.Print("Continue? [y/N] ")
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("Aborted.")
		return nil
	}
	return engine.RunAsRoot("__write-artifact", targetName, device)
}

func runWriteArtifact(_ *cobra.Command, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("private artifact writer requires root")
	}
	targetName := args[0]
	device, err := resolveBlockDevice(args[1])
	if err != nil {
		return err
	}
	project, target, err := loadProjectAndTarget(targetName)
	if err != nil {
		return err
	}
	store := artifact.NewStore(project.TargetBuildDir(targetName), targetName, target.ResolvedPlatform())
	receipt, err := store.Load()
	if err != nil {
		return err
	}
	if err := store.Verify(receipt); err != nil {
		return fmt.Errorf("verifying sealed artifact: %w", err)
	}
	if len(receipt.Partitions) == 0 {
		return fmt.Errorf("artifact receipt has no partition layout; rebuild the target")
	}
	temporary, err := os.MkdirTemp("", "starforge-write-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if _, err := engine.ExtractDiskPartitions(store.DiskPath(), receipt.Partitions, temporary); err != nil {
		return fmt.Errorf("extracting sealed partitions: %w", err)
	}
	output, err := engine.InitOutput("", "write", targetName)
	if err != nil {
		return err
	}
	defer output.Close()
	return output.Run(func() error {
		return engine.WriteToDevice(receipt.Partitions, device, temporary)
	})
}

func resolveBlockDevice(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if strings.HasPrefix(path, "/dev/") {
			return "", fmt.Errorf("device %s not found", path)
		}
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return "", fmt.Errorf("%s is not a block device; use 'starforge export' to create a disk image", path)
	}
	return resolved, nil
}
