package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var writeCmd = &cobra.Command{
	Use:   "write <target> <device>",
	Short: "Write a built target to a storage device",
	Long: `Write a previously built target to a block device (e.g. a USB drive or SD card).

This writes the pre-built partition images from the last build directly to the
device using dd. Growable partitions are expanded to fill available space.

Requires a prior 'starforge build' — this is a write-only operation.`,
	Args: cobra.ExactArgs(2),
	RunE: runWrite,
}

func runWrite(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	device := args[1]

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	_, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	// Validate device path
	if !strings.HasPrefix(device, "/dev/") {
		return fmt.Errorf("device path must start with /dev/: %s", device)
	}
	info, err := os.Stat(device)
	if err != nil {
		return fmt.Errorf("cannot access device %s: %w", device, err)
	}
	if info.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("%s is not a block device", device)
	}

	// Elevate to root before fetching sources
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	// Ensure partition images exist and are up to date
	builder := engine.NewBuilder(proj)
	ctx, err := builder.EnsurePackaged(targetName)
	if err != nil {
		return fmt.Errorf("target not ready: %w", err)
	}
	engine.ChownToInvoker(proj.BuildDir())

	buildDir := proj.TargetBuildDir(targetName)

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
	// These are added post-packaging because payloads reference other targets'
	// build artifacts that aren't part of the overlay.
	if engine.HasInstallerActions(ctx) {
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
	}

	return nil
}
