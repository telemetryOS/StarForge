package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		entryRoot, err := bootEntryRoot(rootfs, entry.Partition)
		if err != nil {
			return fmt.Errorf("boot entry %s: %w", entry.Name, err)
		}
		entryDir := filepath.Join(entryRoot, "loader/entries")
		if err := os.MkdirAll(entryDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", entryDir, err)
		}

		out.Info("entry: %s (%s) -> %s", entry.Name, entry.Title, entryRoot)
		content := fmt.Sprintf("title   %s\nlinux   %s\ninitrd  %s\noptions %s\n",
			entry.Title, entry.Linux, entry.Initrd, entry.Options)
		// systemd-boot ignores entries without a .conf extension.
		name := entry.Name
		if !strings.HasSuffix(name, ".conf") {
			name += ".conf"
		}
		if err := writeFile(filepath.Join(entryDir, name), content); err != nil {
			return fmt.Errorf("writing boot entry %s: %w", entry.Name, err)
		}
	}
	return nil
}

// bootEntryRoot returns the absolute directory under rootfs where a boot
// entry's loader/entries/ tree lives, based on the entry's Partition field.
//
//   ""    /boot   → rootfs/boot   (default; XBOOTLDR if present, else ESP)
//   "esp"         → rootfs/efi    (forces the ESP, used to keep an entry
//                                   on the frozen ESP when XBOOTLDR holds
//                                   the actively-managed entries)
func bootEntryRoot(rootfs, partition string) (string, error) {
	switch partition {
	case "", "boot":
		return filepath.Join(rootfs, "boot"), nil
	case "esp":
		return filepath.Join(rootfs, "efi"), nil
	default:
		return "", fmt.Errorf("unknown partition %q (expected \"\", \"boot\", or \"esp\")", partition)
	}
}

// boolToNo returns "no" for false, "yes" for true.
func boolToNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
