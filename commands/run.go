package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var runSerial bool
var runOverlay string
var runBootDisk string

var runCmd = &cobra.Command{
	Use:   "run <target>",
	Short: "Boot a built target in QEMU",
	Long: `Assemble partition images into a virtual disk via device mapper and boot
in QEMU for testing. Requires a prior 'starforge build'.

The virtual disk is assembled from individual partition images using
device mapper, with a GPT partition table written via sfdisk. QEMU boots
with OVMF UEFI firmware and virtio devices.

SSH is forwarded on port 2222: ssh -p 2222 localhost`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

func init() {
	runCmd.Flags().BoolVar(&runSerial, "serial", false, "attach serial console to terminal")
	runCmd.Flags().StringVar(&runOverlay, "overlay", "", "named overlay for persistent changes")
	runCmd.Flags().StringVar(&runBootDisk, "boot-disk", "", "boot from a named QEMU disk instead of the build target")
}

func runRun(cmd *cobra.Command, args []string) error {
	targetName := args[0]

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	target, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	// Device mapper and losetup need root — elevate before fetching sources
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	buildDir := proj.TargetBuildDir(targetName)

	if runBootDisk != "" {
		// Boot directly from a named disk — no build or device mapper needed
		return engine.RunQEMU(targetName, buildDir, proj.Dir, nil, runSerial, runOverlay, runBootDisk, target.QEMU)
	}

	// Collect to get partition definitions
	builder := engine.NewBuilder(proj)
	ctx, err := builder.Collect(target, false)
	if err != nil {
		return err
	}

	if len(ctx.Partitions) == 0 {
		return fmt.Errorf("target %q has no partitions defined", targetName)
	}

	// Verify partition images exist
	for _, part := range ctx.Partitions {
		imgPath := fmt.Sprintf("%s/%s.img", buildDir, part.Name)
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			return fmt.Errorf("partition image not found: %s — run 'starforge build %s' first", imgPath, targetName)
		}
	}

	// Clean up stale device mapper and loop devices from a previous crashed run
	engine.CleanupAll(buildDir)

	return engine.RunQEMU(targetName, buildDir, proj.Dir, ctx.Partitions, runSerial, runOverlay, runBootDisk, target.QEMU)
}
