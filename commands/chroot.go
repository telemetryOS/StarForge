package commands

import (
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var overlayNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var chrootOverlay string

var chrootCmd = &cobra.Command{
	Use:   "chroot <target> [-- command...]",
	Short: "Open a chroot shell in a built target",
	Long: `Enter the built target filesystem interactively using arch-chroot.

Without a command, opens an interactive shell. With a command after '--',
executes it and returns.

Use --overlay to create a named overlay with persistent changes across
sessions. Without --overlay, changes are discarded on exit.

Requires a prior 'starforge build'.`,
	Args:              cobra.MinimumNArgs(1),
	DisableFlagParsing: false,
	RunE:              runChroot,
}

func init() {
	chrootCmd.Flags().StringVar(&chrootOverlay, "overlay", "", "named overlay for persistent changes")
}

func runChroot(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	chrootArgs := args[1:]

	if chrootOverlay != "" && !overlayNameRe.MatchString(chrootOverlay) {
		return fmt.Errorf("invalid overlay name %q — must match %s", chrootOverlay, overlayNameRe.String())
	}

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	target, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	// Elevate to root — overlay mounts and arch-chroot require it
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	buildDir := proj.TargetBuildDir(targetName)
	os.MkdirAll(buildDir, 0o755)

	output, err := engine.InitOutput(buildDir, "chroot", targetName)
	if err != nil {
		return err
	}
	defer output.Close()

	// Build inside bubbletea
	err = output.Run(func() error {
		builder := engine.NewBuilder(proj)
		return builder.Build(targetName, target, false)
	})
	if err != nil {
		return err
	}

	// Chroot needs direct terminal access (like QEMU)
	result, err := engine.LoadBuildResult(buildDir)
	if err != nil {
		return fmt.Errorf("loading build result: %w", err)
	}

	// Clean up stale mounts
	engine.CleanupAll(buildDir)

	builder := engine.NewBuilder(proj)
	return builder.Chroot(targetName, chrootArgs, chrootOverlay, result.Partitions)
}
