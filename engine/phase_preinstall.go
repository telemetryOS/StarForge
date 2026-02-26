package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phasePreinstall(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Keymap == "" {
		return nil
	}
	// Write vconsole.conf before pacstrap so mkinitcpio's sd-vconsole hook
	// finds it when building the initramfs during linux package installation.
	out.Info("vconsole.conf: KEYMAP=%s", ctx.Keymap)
	if err := os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755); err != nil {
		return fmt.Errorf("creating etc directory: %w", err)
	}
	return writeFile(filepath.Join(rootfs, "etc/vconsole.conf"), fmt.Sprintf("KEYMAP=%s\n", ctx.Keymap))
}
