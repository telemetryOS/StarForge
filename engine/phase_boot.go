package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

func (b *Builder) phaseBoot(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Boot == nil {
		return nil
	}

	// loader.conf goes on the ESP partition's mount point (systemd-boot
	// reads it from where its binary lives — the ESP). Only this target's
	// own systemd-boot-install action creates one if it included a
	// `loader:` block. Embeds typically only contribute entries.
	if ctx.Boot.Loader != nil {
		espPart, ok := findPartitionByType(ctx, "efi")
		if !ok {
			return fmt.Errorf("systemd-boot-install loader: requires a partition with type \"efi\" declared")
		}
		loaderDir := filepath.Join(rootfs, espPart.MountPoint, "loader")
		if err := os.MkdirAll(loaderDir, 0o755); err != nil {
			return fmt.Errorf("creating loader dir: %w", err)
		}

		loader := fmt.Sprintf("default %s\ntimeout %d\neditor %s\n",
			ctx.Boot.Loader.Default,
			ctx.Boot.Loader.Timeout,
			yesNo(ctx.Boot.Loader.Editor))
		out.Info("loader.conf -> %s (default=%s, timeout=%d)",
			espPart.MountPoint, ctx.Boot.Loader.Default, ctx.Boot.Loader.Timeout)
		if err := writeFile(filepath.Join(loaderDir, "loader.conf"), loader); err != nil {
			return fmt.Errorf("writing loader.conf: %w", err)
		}
	}

	for _, entry := range ctx.Boot.Entries {
		if err := b.writeBootEntry(ctx, rootfs, entry); err != nil {
			return fmt.Errorf("boot entry %q: %w", entry.Name, err)
		}
	}
	return nil
}

// writeBootEntry resolves the entry's target partition, stages its kernel and
// initrd if needed, and writes the .conf file with paths relative to the
// partition the entry lives on.
func (b *Builder) writeBootEntry(ctx *actions.BuildContext, rootfs string, entry config.BootEntry) error {
	if entry.Kernel == "" {
		return fmt.Errorf("kernel is required (mkinitcpio kernel name, e.g. \"linux\")")
	}

	// Resolve which partition the entry lives on.
	useExtended := resolveExtended(ctx, entry.Extended)
	wantType := "efi"
	if useExtended {
		wantType = "xbootldr"
	}
	part, ok := findPartitionByType(ctx, wantType)
	if !ok {
		if useExtended {
			return fmt.Errorf("extended: true but no partition with type \"xbootldr\" is declared")
		}
		return fmt.Errorf("no partition with type \"efi\" is declared")
	}

	// Resolve the on-disk directory where the kernel/initrd files should live.
	stageDir := entry.Path
	if stageDir == "" {
		stageDir = part.MountPoint
	}
	stageDir = filepath.Clean(stageDir)
	mountPoint := filepath.Clean(part.MountPoint)
	if !pathHasPrefix(stageDir, mountPoint) {
		return fmt.Errorf("path %q must be a subpath of the entry partition's mount point %q",
			entry.Path, part.MountPoint)
	}

	// The kernel and initrd files must already be at stageDir — typically
	// because this target's pacstrap wrote them there (i.e. the kernel
	// partition is mounted at /boot, the canonical pacman target). The
	// engine does NOT auto-stage from /boot to a different stageDir; layers
	// that need a frozen copy on a different partition should mount that
	// partition at /boot, or copy the files explicitly via file-copy.
	kernelFile := "vmlinuz-" + entry.Kernel
	initrdFile := "initramfs-" + entry.Kernel + ".img"

	if err := requireBootFile(rootfs, stageDir, kernelFile, entry.Kernel); err != nil {
		return err
	}
	if err := requireBootFile(rootfs, stageDir, initrdFile, entry.Kernel); err != nil {
		return err
	}

	// Compute the entry-relative path: strip the partition mount point prefix.
	entryRelDir, err := filepath.Rel(mountPoint, stageDir)
	if err != nil {
		return fmt.Errorf("computing entry-relative path: %w", err)
	}
	entryLinux := "/" + filepath.Join(entryRelDir, kernelFile)
	entryInitrd := "/" + filepath.Join(entryRelDir, initrdFile)
	entryLinux = filepath.Clean(entryLinux)
	entryInitrd = filepath.Clean(entryInitrd)

	// The .conf file goes under <mount_point>/loader/entries/.
	entryDir := filepath.Join(rootfs, mountPoint, "loader/entries")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", entryDir, err)
	}

	out.Info("entry: %s (%s) -> %s [kernel %s]",
		entry.Name, entry.Title, mountPoint, entry.Kernel)

	var content strings.Builder
	content.WriteString(fmt.Sprintf("title   %s\n", entry.Title))
	if entry.SortKey != "" {
		content.WriteString(fmt.Sprintf("sort-key %s\n", entry.SortKey))
	}
	content.WriteString(fmt.Sprintf("linux   %s\ninitrd  %s\noptions %s\n",
		entryLinux, entryInitrd, entry.Options))

	name := entry.Name
	if !strings.HasSuffix(name, ".conf") {
		name += ".conf"
	}
	return writeFile(filepath.Join(entryDir, name), content.String())
}

// resolveExtended returns the effective extended flag for an entry. If the
// entry sets it explicitly, that wins. Otherwise the default is true if any
// xbootldr partition is declared (matches bootctl's behavior of writing
// actively-managed entries to XBOOTLDR), else false.
func resolveExtended(ctx *actions.BuildContext, explicit *bool) bool {
	if explicit != nil {
		return *explicit
	}
	_, hasXbootldr := findPartitionByType(ctx, "xbootldr")
	return hasXbootldr
}

// findPartitionByType locates the first partition with the given GPT type
// (e.g. "efi", "xbootldr") in the build context.
func findPartitionByType(ctx *actions.BuildContext, partType string) (*actions.PartitionDef, bool) {
	for i := range ctx.Partitions {
		if ctx.Partitions[i].Type == partType {
			return &ctx.Partitions[i], true
		}
	}
	return nil, false
}

// pathHasPrefix reports whether path is base or descends from base. Both
// are treated as cleaned absolute-ish paths (e.g. /efi, /efi/recovery).
func pathHasPrefix(path, base string) bool {
	if path == base {
		return true
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return strings.HasPrefix(path, base)
}

// requireBootFile asserts a kernel/initrd file exists at stageDir/file in
// the target's overlay. If absent, returns a clear error. The engine no
// longer copies files from /boot to a different stageDir — the OS layer is
// responsible for arranging that pacman/mkinitcpio writes to where the
// boot entry expects.
func requireBootFile(rootfs, stageDir, file, kernelName string) error {
	dest := filepath.Join(rootfs, stageDir, file)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	return fmt.Errorf(
		"boot file %q not found at %q (mount the partition holding entries at /boot so pacman-add %q writes there directly, or copy the file in via a file-copy action)",
		file, dest, kernelName)
}

// yesNo formats a bool as the literal "yes"/"no" used by systemd-boot's
// loader.conf format.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Deprecated: use yesNo. Kept temporarily for callers in tests until they
// migrate.
func boolToNo(b bool) string { return yesNo(b) }
