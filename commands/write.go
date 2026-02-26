package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var writeCmd = &cobra.Command{
	Use:   "write <target> <output>",
	Short: "Write a built target to a device or disk image",
	Long: `Write a previously built target to a block device or compressed disk image.

For block devices (e.g. /dev/sda), partition images are written directly
using dd and growable partitions are expanded to fill available space.

For file paths, a compressed disk image (.img.gz) is created that can be
flashed with tools like Balena Etcher or Rufus. Parent directories are
created automatically if they don't exist.

Requires a prior 'starforge build' — this is a write-only operation.`,
	Args: cobra.ExactArgs(2),
	RunE: runWrite,
}

func runWrite(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	output := args[1]

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	_, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	// Detect if output is an existing block device
	isDevice := false
	if info, statErr := os.Stat(output); statErr == nil {
		isDevice = info.Mode()&os.ModeDevice != 0
	}

	// Paths under /dev/ that aren't block devices are an error
	if !isDevice && strings.HasPrefix(output, "/dev/") {
		if _, err := os.Stat(output); err != nil {
			return fmt.Errorf("device %s not found", output)
		}
		return fmt.Errorf("%s is not a block device", output)
	}

	// Elevate to root before fetching sources
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	// Ensure partition images exist and are up to date.
	// Auto-builds if no prior build exists.
	builder := engine.NewBuilder(proj)
	ctx, err := builder.EnsureBuiltAndPackaged(targetName)
	if err != nil {
		return err
	}
	engine.ChownToInvoker(proj.BuildDir())

	buildDir := proj.TargetBuildDir(targetName)

	if isDevice {
		return writeToDevice(builder, ctx, buildDir, output)
	}
	return writeToFile(builder, ctx, buildDir, output)
}

// writeToDevice writes partition images directly to a block device.
func writeToDevice(builder *engine.Builder, ctx *actions.BuildContext, buildDir, device string) error {
	// Confirm destruction
	fmt.Printf("WARNING: All data on %s will be destroyed.\n", device)
	fmt.Print("Continue? [y/N] ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("Aborted.")
		return nil
	}

	// Write partition images to device
	if err := engine.WriteToDevice(ctx.Partitions, device, buildDir); err != nil {
		return fmt.Errorf("writing to device: %w", err)
	}

	// Bundle installer components if the target has any installer actions.
	if engine.HasInstallerActions(ctx) {
		if err := bundleInstaller(builder, ctx, device); err != nil {
			return err
		}
	}

	return nil
}

// writeToFile creates a compressed disk image (.img.gz) suitable for
// flashing with tools like Balena Etcher or Rufus.
func writeToFile(builder *engine.Builder, ctx *actions.BuildContext, buildDir, outputPath string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Normalize the output path to ensure it ends with .img.gz:
	//   "foo.img.gz" → raw "foo.img", compressed "foo.img.gz"
	//   "foo.img"    → raw "foo.img", compressed "foo.img.gz"
	//   "foo"        → raw "foo.img", compressed "foo.img.gz"
	rawPath := outputPath
	if trimmed, ok := strings.CutSuffix(rawPath, ".gz"); ok {
		rawPath = trimmed
	}
	if !strings.HasSuffix(rawPath, ".img") {
		rawPath += ".img"
	}

	// Create disk image and write partition images via loop device
	loopDev, cleanup, err := engine.WriteToDiskImage(ctx.Partitions, buildDir, rawPath)
	if err != nil {
		return err
	}

	// Bundle installer components if needed (before detaching loop)
	if engine.HasInstallerActions(ctx) {
		if err := bundleInstaller(builder, ctx, loopDev); err != nil {
			cleanup()
			return err
		}
	}

	// Detach loop device before compression
	cleanup()

	// Compress the raw image with gzip
	gzPath, err := engine.CompressDiskImage(rawPath)
	if err != nil {
		return err
	}

	// Ensure the output file is owned by the invoking user
	engine.ChownToInvoker(gzPath)

	fmt.Println()
	fmt.Printf("Disk image: %s\n", gzPath)
	return nil
}

// bundleInstaller mounts the partitions on a device (or loop device) and
// bundles installer components into the rootfs.
func bundleInstaller(builder *engine.Builder, ctx *actions.BuildContext, device string) error {
	fmt.Println()
	fmt.Printf("  Bundling installer\n")

	rootfs, err := os.MkdirTemp("", "starforge-write-installer-*")
	if err != nil {
		return fmt.Errorf("creating temp mount: %w", err)
	}
	defer os.RemoveAll(rootfs)

	mt := engine.NewMountTable(rootfs)
	var mounts []engine.PartitionMount
	for i, p := range ctx.Partitions {
		mounts = append(mounts, engine.PartitionMount{
			Source:     engine.PartitionPath(device, i+1),
			MountPoint: p.MountPoint,
		})
	}
	if err := mt.MountAll(mounts); err != nil {
		return fmt.Errorf("mounting for installer bundling: %w", err)
	}
	defer mt.Unmount()

	if err := builder.BundleInstallerToRootfs(ctx, rootfs); err != nil {
		return fmt.Errorf("installer bundling: %w", err)
	}

	return nil
}
