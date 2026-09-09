package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/executor"
)

// loadProjectAndTarget loads the project and validates the target name.
func loadProjectAndTarget(targetName string) (*config.Project, config.Target, error) {
	proj, err := config.FindProject()
	if err != nil {
		return nil, config.Target{}, err
	}
	target, ok := proj.Targets[targetName]
	if !ok {
		return nil, config.Target{}, fmt.Errorf("unknown target %q", targetName)
	}
	return proj, target, nil
}

var exportPartitionFormat string

var exportCmd = &cobra.Command{
	Use:   "export <target> <type> [output]",
	Short: "Export sealed build artifacts",
	Long: `Export a target from its sealed full-disk build artifact.

Type must be "disk" or "partitions".

  starforge export device disk ./release/device.img
  starforge export device disk ./release/device.corona --format corona
  starforge export device partitions ./release/
  starforge export device partitions ./release/ --format corona

Disk and partition exports are extracted without mounting the artifact. Use
--format corona for sparse, checksummed Corona output.`,
	Args: cobra.RangeArgs(2, 3),
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportPartitionFormat, "format", "image", "export format: image or corona")
}

func runExport(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	exportType := args[1]
	outputPath := ""
	if len(args) == 3 {
		outputPath = args[2]
	}
	if exportPartitionFormat != "image" && exportPartitionFormat != "corona" {
		return fmt.Errorf("invalid --format %q — must be 'image' or 'corona'", exportPartitionFormat)
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
	output, err := engine.InitOutput(buildDir, "export", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	var artifactResult *executor.ArtifactResult
	if err := output.Run(func() error {
		var buildErr error
		artifactResult, buildErr = buildExecutor.EnsureArtifact(cmd.Context(), executor.BuildRequest{
			Project: project, TargetName: targetName, Target: target,
		})
		return buildErr
	}); err != nil {
		return err
	}

	switch exportType {
	case "disk":
		return exportDisk(cmd.Context(), artifactResult.Disk, buildDir, outputPath)
	case "partitions":
		return exportPartitions(cmd.Context(), artifactResult, outputPath)
	default:
		return fmt.Errorf("unknown export type %q — must be 'disk' or 'partitions'", exportType)
	}
}

func exportDisk(ctx context.Context, artifactDisk, buildDir, outputPath string) error {
	if outputPath == "" {
		if exportPartitionFormat == "corona" {
			outputPath = filepath.Join(buildDir, "disk.corona")
		} else {
			outputPath = filepath.Join(buildDir, "disk.img")
		}
	}
	if exportPartitionFormat == "corona" {
		if !strings.HasSuffix(outputPath, ".corona") {
			outputPath += ".corona"
		}
		if err := engine.EnsureCoronaFile(artifactDisk, outputPath); err != nil {
			return fmt.Errorf("creating Corona file: %w", err)
		}
		engine.OutputSuccess(fmt.Sprintf("Corona file: %s", outputPath))
		return ctx.Err()
	}
	if !strings.HasSuffix(outputPath, ".img") {
		outputPath += ".img"
	}
	if err := engine.CopySparseFile(artifactDisk, outputPath); err != nil {
		return fmt.Errorf("copying disk artifact: %w", err)
	}
	engine.OutputSuccess(fmt.Sprintf("Disk image: %s", outputPath))
	return ctx.Err()
}

func exportPartitions(ctx context.Context, artifactResult *executor.ArtifactResult, outputDir string) error {
	if outputDir == "" {
		outputDir = filepath.Join(artifactResult.Build.BuildDir, "partitions")
	}
	images, err := engine.ExtractDiskPartitions(artifactResult.Disk, artifactResult.Build.Result.Partitions, outputDir)
	if err != nil {
		return err
	}
	for _, image := range images {
		if err := ctx.Err(); err != nil {
			return err
		}
		if exportPartitionFormat == "image" {
			continue
		}
		coronaPath := strings.TrimSuffix(image, ".img") + ".corona"
		if err := engine.EnsureCoronaFile(image, coronaPath); err != nil {
			return fmt.Errorf("creating %s: %w", coronaPath, err)
		}
		if err := os.Remove(image); err != nil {
			return err
		}
	}
	engine.OutputSuccess(fmt.Sprintf("Partitions: %s", outputDir))
	return nil
}
