package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/telemetryos/starforge/actions"
)

// OverlayManager manages overlayfs mounts for incremental builds.
// Each build phase gets its own overlay layer. The upper directory
// captures only the delta for that phase, enabling cache reuse.
type OverlayManager struct {
	cacheDir  string // e.g. .starforge/device/cache/
	mergedDir string // e.g. .starforge/device/merged/
	workDir   string // e.g. .starforge/device/work/
	mounted   bool   // true if an overlay is currently mounted
}

// NewOverlayManager creates an overlay manager for the given build directory.
func NewOverlayManager(buildDir string) *OverlayManager {
	return &OverlayManager{
		cacheDir:  filepath.Join(buildDir, "cache"),
		mergedDir: filepath.Join(buildDir, "merged"),
		workDir:   filepath.Join(buildDir, "work"),
	}
}

// CacheDir returns the path to the cache directory.
func (om *OverlayManager) CacheDir() string {
	return om.cacheDir
}

// MergedDir returns the path to the merged overlay mount point.
func (om *OverlayManager) MergedDir() string {
	return om.mergedDir
}

// Init creates the required directories.
func (om *OverlayManager) Init() error {
	for _, dir := range []string{om.cacheDir, om.mergedDir, om.workDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}
	return nil
}

// PhaseUpperDir returns the upper directory path for a phase.
func (om *OverlayManager) PhaseUpperDir(phaseIndex int) string {
	return filepath.Join(om.cacheDir, PhaseNames[phaseIndex], "upper")
}

// MountPhase mounts an overlay for the given phase.
// All previous phase layers are used as lowerdir, and the new phase
// gets a fresh upperdir to capture its delta.
func (om *OverlayManager) MountPhase(phaseIndex int) error {
	if om.mounted {
		if err := om.unmountOverlay(); err != nil {
			return fmt.Errorf("unmounting previous overlay: %w", err)
		}
	}

	upperDir := om.PhaseUpperDir(phaseIndex)
	if err := os.MkdirAll(upperDir, 0o755); err != nil {
		return fmt.Errorf("creating upper dir: %w", err)
	}

	// Reset work directory (overlayfs requires it to be empty)
	if err := os.RemoveAll(om.workDir); err != nil {
		return fmt.Errorf("cleaning work dir: %w", err)
	}
	if err := os.MkdirAll(om.workDir, 0o755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	// Build lowerdir from all previous cached layers (newest first)
	var lowers []string
	for i := phaseIndex - 1; i >= 0; i-- {
		lower := om.PhaseUpperDir(i)
		if _, err := os.Stat(lower); err == nil {
			lowers = append(lowers, lower)
		}
	}

	// Recreate the merged directory to clear any overlay metadata
	// the kernel may have set on the mount point during previous mounts.
	if err := os.RemoveAll(om.mergedDir); err != nil {
		return fmt.Errorf("cleaning merged dir: %w", err)
	}
	if err := os.MkdirAll(om.mergedDir, 0o755); err != nil {
		return fmt.Errorf("creating merged dir: %w", err)
	}

	var opts string
	if len(lowers) == 0 {
		// First phase: no lower layers. Use an empty tmpdir as lowerdir
		// since overlayfs requires at least a lowerdir.
		emptyDir := filepath.Join(om.cacheDir, "empty")
		if err := os.MkdirAll(emptyDir, 0o755); err != nil {
			return fmt.Errorf("creating empty lower: %w", err)
		}
		opts = fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,metacopy=off",
			emptyDir, upperDir, om.workDir)
	} else {
		opts = fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,metacopy=off",
			strings.Join(lowers, ":"), upperDir, om.workDir)
	}

	if err := unix.Mount("overlay", om.mergedDir, "overlay", 0, opts); err != nil {
		return fmt.Errorf("mounting overlay for phase %d: %w", phaseIndex, err)
	}

	om.mounted = true
	return nil
}

// CommitPhase unmounts the overlay after a phase completes.
// The upper directory retains only the delta from this phase.
func (om *OverlayManager) CommitPhase(phaseIndex int, hash string, manifest *Manifest) error {
	if err := om.unmountOverlay(); err != nil {
		return fmt.Errorf("unmounting phase %d: %w", phaseIndex, err)
	}

	// Strip trusted.overlay.* xattrs so this upper dir can be reused
	// as a lower dir in subsequent phases without mount failures.
	if err := stripOverlayXattrs(om.PhaseUpperDir(phaseIndex)); err != nil {
		return fmt.Errorf("stripping overlay xattrs from phase %d: %w", phaseIndex, err)
	}

	manifest.Phases[PhaseNames[phaseIndex]] = PhaseEntry{
		Hash:      hash,
		Completed: true,
	}

	return manifest.Save(om.cacheDir)
}

// stripOverlayXattrs removes all trusted.overlay.* xattrs from the directory
// tree. The kernel writes these during overlay mounts (e.g. trusted.overlay.origin,
// trusted.overlay.impure) and they prevent the directory from being reused as
// a lower dir in a new overlay mount.
func stripOverlayXattrs(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the size needed for xattr list
		sz, err := unix.Llistxattr(path, nil)
		if err != nil || sz == 0 {
			return nil
		}

		buf := make([]byte, sz)
		sz, err = unix.Llistxattr(path, buf)
		if err != nil || sz == 0 {
			return nil
		}

		// Parse null-terminated xattr names and remove overlay ones
		for _, name := range strings.Split(string(buf[:sz]), "\x00") {
			if strings.HasPrefix(name, "trusted.overlay.") {
				unix.Lremovexattr(path, name)
			}
		}
		return nil
	})
}

// MountMerged mounts all cached phase layers as a single read-only merged view.
// Returns the merged directory path.
func (om *OverlayManager) MountMerged() (string, error) {
	if om.mounted {
		if err := om.unmountOverlay(); err != nil {
			return "", fmt.Errorf("unmounting previous overlay: %w", err)
		}
	}

	// Collect all existing phase upper dirs (oldest first for lowerdir)
	var lowers []string
	for i := len(PhaseNames) - 1; i >= 0; i-- {
		upper := om.PhaseUpperDir(i)
		if _, err := os.Stat(upper); err == nil {
			lowers = append(lowers, upper)
		}
	}

	if len(lowers) == 0 {
		return "", fmt.Errorf("no cached layers found — run build first")
	}

	// Recreate the merged directory to clear any stale overlay metadata.
	if err := os.RemoveAll(om.mergedDir); err != nil {
		return "", fmt.Errorf("cleaning merged dir: %w", err)
	}
	if err := os.MkdirAll(om.mergedDir, 0o755); err != nil {
		return "", fmt.Errorf("creating merged dir: %w", err)
	}

	// Read-only mount: all layers as lowerdir, no upperdir/workdir
	opts := fmt.Sprintf("lowerdir=%s", strings.Join(lowers, ":"))

	if err := unix.Mount("overlay", om.mergedDir, "overlay", 0, opts); err != nil {
		return "", fmt.Errorf("mounting merged overlay: %w", err)
	}

	om.mounted = true
	return om.mergedDir, nil
}

// MountMergedWritable mounts all cached layers with a temporary upper directory,
// providing a writable view of the full filesystem. Changes are discarded on unmount.
// Used by chroot to allow arch-chroot bind mounts and interactive modifications.
func (om *OverlayManager) MountMergedWritable() (string, error) {
	if om.mounted {
		if err := om.unmountOverlay(); err != nil {
			return "", fmt.Errorf("unmounting previous overlay: %w", err)
		}
	}

	// Collect all existing phase upper dirs (oldest first for lowerdir)
	var lowers []string
	for i := len(PhaseNames) - 1; i >= 0; i-- {
		upper := om.PhaseUpperDir(i)
		if _, err := os.Stat(upper); err == nil {
			lowers = append(lowers, upper)
		}
	}

	if len(lowers) == 0 {
		return "", fmt.Errorf("no cached layers found — run build first")
	}

	// Temporary upper/work dirs for the writable session (discarded on unmount)
	chrootUpper := filepath.Join(om.cacheDir, "chroot-upper")
	if err := os.RemoveAll(chrootUpper); err != nil {
		return "", fmt.Errorf("cleaning chroot upper: %w", err)
	}
	if err := os.MkdirAll(chrootUpper, 0o755); err != nil {
		return "", fmt.Errorf("creating chroot upper: %w", err)
	}

	if err := os.RemoveAll(om.workDir); err != nil {
		return "", fmt.Errorf("cleaning work dir: %w", err)
	}
	if err := os.MkdirAll(om.workDir, 0o755); err != nil {
		return "", fmt.Errorf("creating work dir: %w", err)
	}

	// Recreate merged dir to clear stale kernel metadata
	if err := os.RemoveAll(om.mergedDir); err != nil {
		return "", fmt.Errorf("cleaning merged dir: %w", err)
	}
	if err := os.MkdirAll(om.mergedDir, 0o755); err != nil {
		return "", fmt.Errorf("creating merged dir: %w", err)
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,metacopy=off",
		strings.Join(lowers, ":"), chrootUpper, om.workDir)

	if err := unix.Mount("overlay", om.mergedDir, "overlay", 0, opts); err != nil {
		return "", fmt.Errorf("mounting writable overlay: %w", err)
	}

	om.mounted = true
	return om.mergedDir, nil
}

// CleanChroot removes the temporary chroot upper directory.
func (om *OverlayManager) CleanChroot() {
	os.RemoveAll(filepath.Join(om.cacheDir, "chroot-upper"))
}

// Unmount unmounts any active overlay.
func (om *OverlayManager) Unmount() error {
	if !om.mounted {
		return nil
	}
	return om.unmountOverlay()
}

func (om *OverlayManager) unmountOverlay() error {
	om.mounted = false

	if err := run("umount", "-R", om.mergedDir); err == nil {
		return nil
	}

	// Busy mount (e.g. stale process from arch-chroot) — use lazy
	// unmount to detach immediately. The mount is cleaned up once
	// the holding process exits.
	return run("umount", "-Rl", om.mergedDir)
}

// EnsureNamedOverlay creates or reuses a named overlay directory with copies
// of the build cache partition images. Returns the overlay directory path.
func EnsureNamedOverlay(buildDir, overlayName string, parts []actions.PartitionDef) (string, error) {
	overlayDir := filepath.Join(buildDir, "overlays", overlayName)
	if err := os.MkdirAll(overlayDir, 0o755); err != nil {
		return "", fmt.Errorf("creating overlay directory: %w", err)
	}

	// Find the latest cache upper directory containing partition images
	om := NewOverlayManager(buildDir)
	var sourceDir string
	for i := len(PhaseNames) - 1; i >= 0; i-- {
		upper := om.PhaseUpperDir(i)
		if _, err := os.Stat(upper); err == nil {
			sourceDir = upper
			break
		}
	}
	if sourceDir == "" {
		return "", fmt.Errorf("no cached build layers found — run build first")
	}

	// Copy partition images that don't already exist in the overlay
	for _, part := range parts {
		imgName := fmt.Sprintf("%s.img", part.Name)
		dest := filepath.Join(overlayDir, imgName)
		if _, err := os.Stat(dest); err == nil {
			continue // already exists
		}

		src := filepath.Join(sourceDir, imgName)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			// Try build dir directly
			src = filepath.Join(buildDir, imgName)
			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}
		}

		if err := CopyFile(src, dest); err != nil {
			return "", fmt.Errorf("copying %s to overlay: %w", imgName, err)
		}
	}

	return overlayDir, nil
}

// InvalidateOverlays removes all named overlays for a target.
func InvalidateOverlays(buildDir string) error {
	overlaysDir := filepath.Join(buildDir, "overlays")
	if _, err := os.Stat(overlaysDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(overlaysDir)
}

// CleanCache removes the entire cache directory for a fresh rebuild.
func (om *OverlayManager) CleanCache() error {
	if om.mounted {
		om.unmountOverlay()
	}
	if err := os.RemoveAll(om.cacheDir); err != nil {
		return fmt.Errorf("removing cache: %w", err)
	}
	if err := os.RemoveAll(om.mergedDir); err != nil {
		return fmt.Errorf("removing merged: %w", err)
	}
	if err := os.RemoveAll(om.workDir); err != nil {
		return fmt.Errorf("removing work: %w", err)
	}
	return nil
}

