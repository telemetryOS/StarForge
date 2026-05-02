package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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

var exportPartitionFormat string

var exportCmd = &cobra.Command{
	Use:   "export <target> <type> [output]",
	Short: "Export build artifacts as images",
	Long: `Export a target as disk images. The target is built automatically if needed.

Type must be "disk" or "partitions".

  starforge export device disk ./release/device.img
  starforge export device disk ./release/device.corona --format corona
  starforge export device partitions ./release/
  starforge export device partitions ./release/ --format corona

Use "disk" to create a single bootable disk image with a GPT partition table.
Use "partitions" to produce individual partition image files. Use --format
corona to create Corona files instead.`,
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

	proj, target, err := loadProjectAndTarget(targetName)
	if err != nil {
		return err
	}

	switch exportType {
	case "disk":
		return runExportDisk(proj, targetName, target, outputPath)
	case "partitions":
		return runExportPartitions(proj, targetName, target, outputPath)
	default:
		return fmt.Errorf("unknown export type %q — must be 'disk' or 'partitions'", exportType)
	}
}

func runExportDisk(proj *config.Project, targetName string, _ config.Target, outputPath string) error {
	switch exportPartitionFormat {
	case "image", "corona":
	default:
		return fmt.Errorf("invalid --format %q — must be 'image' or 'corona'", exportPartitionFormat)
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
		builder := engine.NewBuilder(proj)
		ctx, err := builder.EnsureBuiltAndPackaged(targetName)
		if err != nil {
			return err
		}

		if len(ctx.Partitions) == 0 {
			return fmt.Errorf("target %q has no partitions defined", targetName)
		}

		if outputPath == "" {
			if exportPartitionFormat == "corona" {
				outputPath = filepath.Join(buildDir, "disk.corona")
			} else {
				outputPath = filepath.Join(buildDir, "disk.img")
			}
		}

		var rawPath string
		if exportPartitionFormat == "corona" {
			rawPath = strings.TrimSuffix(outputPath, ".corona")
			if !strings.HasSuffix(rawPath, ".img") {
				rawPath += ".img"
			}
			if !strings.HasSuffix(outputPath, ".corona") {
				outputPath += ".corona"
			}
		} else {
			rawPath = outputPath
			if !strings.HasSuffix(rawPath, ".img") {
				rawPath += ".img"
			}
		}
		if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		loopDev, cleanup, err := engine.WriteToDiskImage(ctx.Partitions, buildDir, rawPath)
		if err != nil {
			return err
		}

		if engine.HasInstallerActions(ctx) {
			if err := bundleInstaller(builder, ctx, loopDev); err != nil {
				cleanup()
				os.Remove(rawPath)
				return err
			}
		}

		cleanup()

		if exportPartitionFormat == "corona" {
			if err := engine.EnsureCoronaFile(rawPath, outputPath); err != nil {
				return fmt.Errorf("creating Corona file: %w", err)
			}
			if err := os.Remove(rawPath); err != nil {
				return fmt.Errorf("removing raw disk image: %w", err)
			}
			engine.ChownToInvoker(outputPath)
			engine.OutputSuccess(fmt.Sprintf("Corona file: %s", outputPath))
		} else {
			engine.ChownToInvoker(rawPath)
			engine.OutputSuccess(fmt.Sprintf("Disk image: %s", rawPath))
		}

		// Ensure build dir is owned by the invoking user
		engine.ChownToInvoker(proj.BuildDir())

		return nil
	})
}

func runExportPartitions(proj *config.Project, targetName string, target config.Target, outputDir string) error {
	switch exportPartitionFormat {
	case "image", "corona":
	default:
		return fmt.Errorf("invalid --format %q — must be 'image' or 'corona'", exportPartitionFormat)
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
		// Incremental build + packaging. This path must use EnsurePackaged
		// because it owns multi-target packaging and installer artifact
		// bundling; PackageToImages is only the legacy single-target primitive.
		builder := engine.NewBuilder(proj)
		ctx, err := builder.EnsureBuiltAndPackaged(targetName)
		if err != nil {
			return err
		}

		if len(ctx.Partitions) == 0 {
			return fmt.Errorf("target %q has no partitions defined", targetName)
		}

		if outputDir != "" {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}
		}

		for _, part := range ctx.Partitions {
			imgName := fmt.Sprintf("%s.img", part.Name)
			src := filepath.Join(buildDir, imgName)

			if _, err := os.Stat(src); err != nil {
				return fmt.Errorf("partition image %s not found — run 'starforge build' first: %w", imgName, err)
			}

			if exportPartitionFormat == "image" {
				if outputDir != "" {
					dest := filepath.Join(outputDir, imgName)
					if err := engine.CopyFile(src, dest); err != nil {
						return fmt.Errorf("copying %s: %w", imgName, err)
					}
				}
				continue
			}

			coronaName := fmt.Sprintf("%s.corona", part.Name)
			srcCorona := filepath.Join(buildDir, coronaName)
			if err := engine.EnsureCoronaFile(src, srcCorona); err != nil {
				return fmt.Errorf("creating Corona file for %s: %w", imgName, err)
			}
			if outputDir != "" {
				destCorona := filepath.Join(outputDir, coronaName)
				if err := engine.CopyFile(srcCorona, destCorona); err != nil {
					return fmt.Errorf("copying %s: %w", coronaName, err)
				}
			}
		}

		// Ensure build dir is owned by the invoking user
		engine.ChownToInvoker(proj.BuildDir())

		return nil
	})
}
