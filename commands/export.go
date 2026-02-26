package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
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

var exportDiskOutput string
var exportDiskSize string

var exportCmd = &cobra.Command{
	Use:   "export <target> <type>",
	Short: "Export build artifacts as images",
	Long: `Export a previously built target as disk images.

Type must be "disk" or "partitions".

  starforge export device disk --size 8G
  starforge export device partitions --output ./release/

Use "disk" to create a single bootable disk image with a GPT partition table.
Use "partitions" to produce individual partition image files.

Requires a prior 'starforge build'.`,
	Args: cobra.ExactArgs(2),
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportDiskSize, "size", "", "total disk image size for 'disk' type (e.g. 8G, 16G)")
	exportCmd.Flags().StringVar(&exportDiskOutput, "output", "", "output path: file for 'disk', directory for 'partitions'")
}

func runExport(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	exportType := args[1]

	proj, target, err := loadProjectAndTarget(targetName)
	if err != nil {
		return err
	}

	switch exportType {
	case "disk":
		return runExportDisk(proj, targetName, target)
	case "partitions":
		return runExportPartitions(proj, targetName, target)
	default:
		return fmt.Errorf("unknown export type %q — must be 'disk' or 'partitions'", exportType)
	}
}

func runExportDisk(proj *config.Project, targetName string, target config.Target) error {
	if exportDiskSize == "" {
		return fmt.Errorf("--size is required for disk export (e.g. --size 8G)")
	}

	// Parse size flag
	diskSize, _, err := actions.ParseSize(exportDiskSize)
	if err != nil {
		return fmt.Errorf("invalid --size: %w", err)
	}

	// Elevate to root before building
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	buildDir := proj.TargetBuildDir(targetName)
	os.MkdirAll(buildDir, 0o755)

	output, err := engine.InitOutput(buildDir, "export", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	return output.Run(func() error {
		// Incremental build — detects source changes via cache hashing
		builder := engine.NewBuilder(proj)
		if err := builder.Build(targetName, target, false); err != nil {
			return err
		}

		result, err := engine.LoadBuildResult(buildDir)
		if err != nil {
			return fmt.Errorf("loading build result: %w", err)
		}

		if len(result.Partitions) == 0 {
			return fmt.Errorf("target %q has no partitions defined", targetName)
		}

		// Validate disk size fits all partitions
		var totalFixed uint64
		for _, p := range result.Partitions {
			totalFixed += p.Size
		}
		if diskSize < totalFixed {
			return fmt.Errorf("disk size %s is too small for partitions (need at least %s)",
				actions.FormatSize(diskSize), actions.FormatSize(totalFixed))
		}

		// Determine output path
		outputPath := exportDiskOutput
		if outputPath == "" {
			outputPath = filepath.Join(buildDir, "disk.img")
		}

		// Clean up any stale mounts from a previous interrupted build
		engine.CleanupAll(buildDir)

		// Mount cached overlay layers as read-only merged view
		overlay := engine.NewOverlayManager(buildDir)
		mergedDir, err := overlay.MountMerged()
		if err != nil {
			return fmt.Errorf("mounting overlay: %w", err)
		}
		defer overlay.Unmount()

		// Create disk image — SetupDevicePartitions handles GPT overhead and
		// growable partition resolution internally.
		if err := engine.PackageToDiskImage(mergedDir, result.Partitions, diskSize, outputPath, engine.PackageOps{
			Ownerships:  result.Ownerships,
			Permissions: result.Permissions,
		}); err != nil {
			return fmt.Errorf("creating disk image: %w", err)
		}

		// Ensure build dir is owned by the invoking user
		engine.ChownToInvoker(proj.BuildDir())

		return nil
	})
}

func runExportPartitions(proj *config.Project, targetName string, target config.Target) error {
	// Elevate to root before building
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	buildDir := proj.TargetBuildDir(targetName)
	os.MkdirAll(buildDir, 0o755)

	output, err := engine.InitOutput(buildDir, "export", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	return output.Run(func() error {
		// Incremental build — detects source changes via cache hashing
		builder := engine.NewBuilder(proj)
		if err := builder.Build(targetName, target, false); err != nil {
			return err
		}

		result, err := engine.LoadBuildResult(buildDir)
		if err != nil {
			return fmt.Errorf("loading build result: %w", err)
		}

		if len(result.Partitions) == 0 {
			return fmt.Errorf("target %q has no partitions defined", targetName)
		}

		// Clean up any stale mounts from a previous interrupted build
		engine.CleanupAll(buildDir)

		// Mount cached overlay layers as read-only merged view
		overlay := engine.NewOverlayManager(buildDir)
		mergedDir, err := overlay.MountMerged()
		if err != nil {
			return fmt.Errorf("mounting overlay: %w", err)
		}
		defer overlay.Unmount()

		// Package into individual partition images
		if err := engine.PackageToImages(mergedDir, result.Partitions, buildDir, engine.PackageOps{
			Ownerships:  result.Ownerships,
			Permissions: result.Permissions,
		}); err != nil {
			return fmt.Errorf("packaging partitions: %w", err)
		}

		// Copy to output directory if specified
		outputDir := exportDiskOutput // reuse --output flag
		if outputDir != "" {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			for _, part := range result.Partitions {
				imgName := fmt.Sprintf("%s.img", part.Name)
				src := filepath.Join(buildDir, imgName)
				dest := filepath.Join(outputDir, imgName)

				if _, err := os.Stat(src); os.IsNotExist(err) {
					continue
				}

				if err := engine.CopyFile(src, dest); err != nil {
					return fmt.Errorf("copying %s: %w", imgName, err)
				}
			}
		}

		// Ensure build dir is owned by the invoking user
		engine.ChownToInvoker(proj.BuildDir())

		return nil
	})
}
