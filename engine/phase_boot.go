package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseBoot(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Boot == nil {
		return nil
	}

	loaderDir := filepath.Join(rootfs, "boot/loader")
	entriesDir := filepath.Join(loaderDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return fmt.Errorf("creating boot loader directories: %w", err)
	}

	loader := fmt.Sprintf("default %s\ntimeout %d\neditor %s\n",
		ctx.Boot.Loader.Default,
		ctx.Boot.Loader.Timeout,
		boolToNo(ctx.Boot.Loader.Editor))
	out.Info("loader.conf (default=%s, timeout=%d)",
		ctx.Boot.Loader.Default, ctx.Boot.Loader.Timeout)
	if err := writeFile(filepath.Join(loaderDir, "loader.conf"), loader); err != nil {
		return fmt.Errorf("writing loader.conf: %w", err)
	}

	for _, entry := range ctx.Boot.Entries {
		out.Info("entry: %s (%s)", entry.Name, entry.Title)
		content := fmt.Sprintf("title   %s\nlinux   %s\ninitrd  %s\noptions %s\n",
			entry.Title, entry.Linux, entry.Initrd, entry.Options)
		if err := writeFile(filepath.Join(entriesDir, entry.Name), content); err != nil {
			return fmt.Errorf("writing boot entry %s: %w", entry.Name, err)
		}
	}
	return nil
}

// boolToNo returns "no" for false, "yes" for true.
func boolToNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
