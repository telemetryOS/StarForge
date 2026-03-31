package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/engine"
)

var runSerial bool
var runOverlay string
var runBootDisk string

var runCmd = &cobra.Command{
	Use:   "run <target>",
	Short: "Boot a target in QEMU",
	Long: `Assemble partition images into a virtual disk via device mapper and boot
in QEMU for testing. The target is built automatically if needed (incremental build).

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

	proj, target, err := loadProjectAndTarget(targetName)
	if err != nil {
		return err
	}

	buildDir := proj.TargetBuildDir(targetName)

	if runBootDisk != "" {
		// Boot directly from a named disk — no build or device mapper needed.
		// Initialize output for QEMU info messages (runs outside bubbletea).
		output, err := engine.InitOutput(buildDir, "run", targetName)
		if err != nil {
			return err
		}
		defer output.Close()
		return engine.RunQEMU(targetName, buildDir, proj.Dir, nil, runSerial, runOverlay, runBootDisk, target.QEMU)
	}

	os.MkdirAll(buildDir, 0o755)

	output, err := engine.InitOutput(buildDir, "run", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	// Build and package inside bubbletea; QEMU runs after.
	var ctx *actions.BuildContext
	err = output.Run(func() error {
		builder := engine.NewBuilder(proj)
		var buildErr error
		ctx, buildErr = builder.EnsureBuiltAndPackaged(targetName)
		if buildErr != nil {
			return buildErr
		}
		engine.ChownToInvoker(proj.BuildDir())
		return nil
	})
	if err != nil {
		return err
	}

	// Bubbletea is done — QEMU runs with direct terminal access.
	// out.* methods still work (fall back to fmt.Println).
	return engine.RunQEMU(targetName, buildDir, proj.Dir, ctx.Partitions, runSerial, runOverlay, runBootDisk, target.QEMU)
}
