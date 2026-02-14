package commands

import (
	"fmt"

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

	// Elevate early — device mapper, losetup, and potential auto-build all need root
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	target, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	buildDir := proj.TargetBuildDir(targetName)

	if runBootDisk != "" {
		// Boot directly from a named disk — no build or device mapper needed
		return engine.RunQEMU(targetName, buildDir, proj.Dir, nil, runSerial, runOverlay, runBootDisk, target.QEMU)
	}

	// Load partition layout saved by the build — if missing, build first
	parts, err := engine.LoadPartitions(buildDir)
	if err != nil {
		fmt.Println("No previous build found, building first...")
		builder := engine.NewBuilder(proj)
		if err := builder.Build(targetName, target, false); err != nil {
			return err
		}
		engine.ChownToInvoker(proj.BuildDir())

		parts, err = engine.LoadPartitions(buildDir)
		if err != nil {
			return err
		}
	}

	return engine.RunQEMU(targetName, buildDir, proj.Dir, parts, runSerial, runOverlay, runBootDisk, target.QEMU)
}
