package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
)

// Chroot enters the built filesystem interactively, or runs a command inside it.
// If args is empty, an interactive shell is started.
//
// When overlayName is set, partition images from a named overlay are loop-mounted
// and changes persist across sessions. When empty, the ephemeral overlayfs
// behavior is used (changes discarded on exit).
func (b *Builder) Chroot(targetName string, args []string, overlayName string, parts []actions.PartitionDef) error {
	buildDir := b.project.TargetBuildDir(targetName)

	if overlayName != "" {
		return b.chrootOverlay(targetName, buildDir, args, overlayName, parts)
	}

	overlay := NewOverlayManager(buildDir)
	manifest, err := LoadManifest(overlay.CacheDir())
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Check that at least one phase has been built
	if len(manifest.Phases) == 0 {
		return fmt.Errorf("target %q has not been built yet — run 'starforge build %s' first", targetName, targetName)
	}

	fmt.Println(headerStyle.Render(fmt.Sprintf("Entering chroot: %s", targetName)))

	// Clean up any stale mounts from a previous session
	CleanupMounts(buildDir)

	// Mount all cached layers as a writable overlay (changes are discarded on unmount)
	mergedDir, err := overlay.MountMergedWritable()
	if err != nil {
		return fmt.Errorf("mounting overlay: %w", err)
	}
	defer func() {
		overlay.Unmount()
		overlay.CleanChroot()
	}()

	cmdArgs := append([]string{mergedDir}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// chrootOverlay enters a chroot using loop-mounted partition images from a named overlay.
func (b *Builder) chrootOverlay(targetName, buildDir string, args []string, overlayName string, parts []actions.PartitionDef) error {
	fmt.Println(headerStyle.Render(fmt.Sprintf("Entering chroot: %s (overlay: %s)", targetName, overlayName)))

	overlayDir, err := EnsureNamedOverlay(buildDir, overlayName, parts)
	if err != nil {
		return fmt.Errorf("named overlay: %w", err)
	}

	// Build partition mounts from overlay images (skip swap)
	var mounts []PartitionMount
	for _, part := range parts {
		if part.Filesystem == "swap" {
			continue
		}
		mounts = append(mounts, PartitionMount{
			Source:     filepath.Join(overlayDir, fmt.Sprintf("%s.img", part.Name)),
			MountPoint: part.MountPoint,
			Loop:       true,
		})
	}

	if len(mounts) == 0 {
		return fmt.Errorf("no mountable partitions found")
	}

	// Create temp dir as mount root
	rootfs, err := os.MkdirTemp("", "starforge-chroot-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(rootfs)

	mt := NewMountTable(rootfs)
	if err := mt.MountAll(mounts); err != nil {
		mt.Unmount()
		return fmt.Errorf("mounting partitions: %w", err)
	}
	defer mt.Unmount()

	cmdArgs := append([]string{rootfs}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
