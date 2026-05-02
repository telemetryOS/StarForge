package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var writeCmd = &cobra.Command{
	Use:   "write <target> <output>",
	Short: "Write a target to a block device",
	Long: `Write a target to a block device.
The target is built automatically if needed.

For block devices (e.g. /dev/sda), partition images are written directly
through the Corona writer and growable partitions are expanded to fill
available space. Use "starforge export" to create disk image files.`,
	Args: cobra.ExactArgs(2),
	RunE: runWrite,
}

func runWrite(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	outputPath := args[1]

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
	if info, statErr := os.Stat(outputPath); statErr == nil {
		mode := info.Mode()
		isDevice = mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0
	}

	// Paths under /dev/ that aren't block devices are an error
	if !isDevice && strings.HasPrefix(outputPath, "/dev/") {
		if _, err := os.Stat(outputPath); err != nil {
			return fmt.Errorf("device %s not found", outputPath)
		}
		return fmt.Errorf("%s is not a block device", outputPath)
	}
	if !isDevice {
		return fmt.Errorf("%s is not a block device; use 'starforge export %s disk %s' to create a disk image", outputPath, targetName, outputPath)
	}

	// Elevate to root before fetching sources
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	// Confirmation for device writes (after elevation so we only ask once)
	fmt.Printf("WARNING: All data on %s will be destroyed.\n", outputPath)
	fmt.Print("Continue? [y/N] ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("Aborted.")
		return nil
	}

	buildDir := proj.TargetBuildDir(targetName)
	os.MkdirAll(buildDir, 0o755)

	output, err := engine.InitOutput(buildDir, "write", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	return output.Run(func() error {
		// Ensure partition images exist and are up to date.
		builder := engine.NewBuilder(proj)
		ctx, err := builder.EnsureBuiltAndPackaged(targetName)
		if err != nil {
			return err
		}
		engine.ChownToInvoker(proj.BuildDir())

		return writeToDevice(builder, ctx, buildDir, outputPath)
	})
}

// writeToDevice writes partition images directly to a block device.
// Confirmation is handled by the caller before entering bubbletea.
func writeToDevice(builder *engine.Builder, ctx *actions.BuildContext, buildDir, device string) error {
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

// bundleInstaller mounts the partitions on a device (or loop device) and
// bundles installer components into the rootfs.
func bundleInstaller(builder *engine.Builder, ctx *actions.BuildContext, device string) error {
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
