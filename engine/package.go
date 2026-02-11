package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/telemetryos/starforge/actions"
)

// PackageToImages creates sparse partition images, formats them, and copies
// the merged overlay tree into the appropriate partitions.
func PackageToImages(mergedDir string, parts []actions.PartitionDef, buildDir string) error {
	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render("Packaging images"))

	// Clean up stale rootfs mounts from previous packaging runs.
	// Scope to rootfs/ only — buildDir/ also contains the merged overlay
	// which must stay mounted as the copy source.
	rootfs := filepath.Join(buildDir, "rootfs")
	CleanupMounts(rootfs)

	// Detach stale loop devices on partition images (e.g. from a
	// previous installer bundling that failed mid-way).
	cleanupLoops(buildDir)

	// Create and format image files
	mounts, err := SetupImagePartitions(parts, buildDir)
	if err != nil {
		return fmt.Errorf("creating images: %w", err)
	}

	return packagePipeline(mergedDir, parts, mounts, rootfs, "images")
}

// WriteToDevice partitions a block device and writes pre-built partition
// images directly using dd with oflag=direct (bypasses kernel page cache).
// Growable ext4 partitions are expanded to fill the device partition.
// This replaces PackageToDevice — no overlay mount or tar copy is needed
// because the images already contain the complete filesystem.
func WriteToDevice(parts []actions.PartitionDef, device, buildDir string) error {
	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render("Writing to device"))

	// Partition the device (GPT via sfdisk)
	resolved, err := PartitionDevice(parts, device)
	if err != nil {
		return fmt.Errorf("partitioning device: %w", err)
	}

	// Write each partition image to the device
	for i, part := range resolved {
		partDev := partitionPath(device, i+1)
		imgPath := filepath.Join(buildDir, fmt.Sprintf("%s.img", part.Name))

		fmt.Printf("    dd %s -> %s\n", part.Name, partDev)
		if err := run("dd", "if="+imgPath, "of="+partDev, "bs=4M", "oflag=direct"); err != nil {
			return fmt.Errorf("writing %s: %w", part.Name, err)
		}

		// Expand filesystem if the device partition is larger than the image
		if resolved[i].Size > parts[i].Size {
			fmt.Printf("    resize %s (%s -> %s)\n", part.Name,
				actions.FormatSize(parts[i].Size), actions.FormatSize(resolved[i].Size))
			if err := expandFilesystem(partDev, part.Filesystem); err != nil {
				return fmt.Errorf("expanding %s: %w", part.Name, err)
			}
		}
	}

	return nil
}

// expandFilesystem grows a filesystem to fill its partition.
func expandFilesystem(partDev, filesystem string) error {
	switch filesystem {
	case "ext4":
		if err := run("e2fsck", "-f", "-y", partDev); err != nil {
			return fmt.Errorf("e2fsck: %w", err)
		}
		if err := run("resize2fs", partDev); err != nil {
			return fmt.Errorf("resize2fs: %w", err)
		}
	}
	return nil
}

// PackageToDiskImage creates a single disk image file with a GPT partition table,
// formats partitions, and copies the merged overlay tree into them.
func PackageToDiskImage(mergedDir string, parts []actions.PartitionDef, diskSize uint64, outputPath string) error {
	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render("Creating disk image"))

	// Create sparse file
	sizeLabel := actions.FormatSize(diskSize)
	fmt.Printf("    %s (%s)\n", outputPath, sizeLabel)

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating disk image: %w", err)
	}
	if err := f.Truncate(int64(diskSize)); err != nil {
		f.Close()
		return fmt.Errorf("setting disk image size: %w", err)
	}
	f.Close()

	// Attach loop device
	loopDev, err := runOutput("losetup", "--find", "--show", outputPath)
	if err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("attaching loop device: %w", err)
	}
	fmt.Printf("    loop device: %s\n", loopDev)

	// Detach loop device on exit
	defer func() {
		fmt.Printf("    detaching %s\n", loopDev)
		run("losetup", "-d", loopDev)
	}()

	// Partition and format via the loop device (it's a block device)
	mounts, err := SetupDevicePartitions(parts, loopDev)
	if err != nil {
		return fmt.Errorf("setting up partitions: %w", err)
	}

	// Mount partitions under a temp dir
	rootfs, err := os.MkdirTemp("", "starforge-disk-*")
	if err != nil {
		return fmt.Errorf("creating temp mount: %w", err)
	}
	defer os.RemoveAll(rootfs)

	return packagePipeline(mergedDir, parts, mounts, rootfs, "disk image")
}

// packagePipeline is the common pipeline shared by all Package functions.
// It verifies the overlay, mounts partitions, copies content, ensures chroot
// dirs, installs the bootloader, generates fstab, and unmounts.
func packagePipeline(mergedDir string, parts []actions.PartitionDef, mounts []PartitionMount, rootfs, label string) error {
	// Verify overlay is mounted and populated before we start
	if err := verifyOverlay(mergedDir); err != nil {
		return fmt.Errorf("source overlay: %w", err)
	}

	// Mount partitions
	mt := NewMountTable(rootfs)

	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render(fmt.Sprintf("Mounting %s", label)))
	if err := mt.MountAll(mounts); err != nil {
		mt.Unmount()
		return fmt.Errorf("mounting %s: %w", label, err)
	}
	defer func() {
		fmt.Println()
		fmt.Printf("  %s\n", phaseStyle.Render(fmt.Sprintf("Unmounting %s", label)))
		mt.Unmount()
	}()

	// Copy merged tree into partitions
	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render(fmt.Sprintf("Copying to %s", label)))
	if err := copyToPartitions(mergedDir, parts, rootfs); err != nil {
		return err
	}

	// Ensure chroot directories exist for bootloader installation
	if err := EnsureChrootDirs(rootfs); err != nil {
		return err
	}

	// Install bootloader now that /boot is a real mounted ESP
	if err := InstallBootloader(parts, rootfs); err != nil {
		return err
	}

	// Generate /etc/fstab with UUIDs from the formatted partitions
	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render("Generating fstab"))
	if err := GenerateFstab(rootfs); err != nil {
		return err
	}

	return nil
}

// verifyOverlay checks that the overlay at mergedDir is actually mounted
// and populated. If mergedDir and its parent have the same device ID,
// the overlay isn't mounted. Also checks that /usr and /etc exist as a
// sanity check that we're looking at a real rootfs.
func verifyOverlay(mergedDir string) error {
	var mergedStat, parentStat unix.Stat_t
	if err := unix.Stat(mergedDir, &mergedStat); err != nil {
		return fmt.Errorf("overlay not accessible: %w", err)
	}
	parentDir := filepath.Dir(mergedDir)
	if err := unix.Stat(parentDir, &parentStat); err != nil {
		return fmt.Errorf("overlay parent not accessible: %w", err)
	}
	if mergedStat.Dev == parentStat.Dev {
		return fmt.Errorf("overlay at %s is not mounted (same device as parent)", mergedDir)
	}

	for _, dir := range []string{"usr", "etc"} {
		if _, err := os.Stat(filepath.Join(mergedDir, dir)); err != nil {
			return fmt.Errorf("overlay at %s appears empty (missing /%s)", mergedDir, dir)
		}
	}
	return nil
}

// EnsureChrootDirs creates directories needed by arch-chroot for bind mounts.
// These should exist from pacstrap, but overlay-to-partition copy may lose
// empty dirs.
func EnsureChrootDirs(rootfs string) error {
	for _, dir := range []string{"proc", "sys", "dev", "run", "tmp"} {
		if err := os.MkdirAll(filepath.Join(rootfs, dir), 0o755); err != nil {
			return fmt.Errorf("creating /%s: %w", dir, err)
		}
	}
	return nil
}

// copyToPartitions copies the relevant subtree from mergedDir into each
// partition's mount point under rootfs, using tar for precise nested path
// exclusion and correct filesystem semantics.
func copyToPartitions(mergedDir string, parts []actions.PartitionDef, rootfs string) error {
	// Sort partitions by mount point depth (/ first, then /boot, /var/log, etc.)
	sorted := make([]actions.PartitionDef, len(parts))
	copy(sorted, parts)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].MountPoint) < len(sorted[j].MountPoint)
	})

	for _, part := range sorted {
		if err := CopyPartition(mergedDir, part, parts, rootfs); err != nil {
			return err
		}
	}

	return nil
}

// DescendantMountPaths returns relative paths for all descendant partitions
// under parentMP. For parent "/" with child "/var/log", returns ["var/log"].
// Unlike the old childMountPoints, this does NOT truncate to the top-level
// component, so tar --exclude can precisely exclude nested paths.
func DescendantMountPaths(parentMP string, parts []actions.PartitionDef) []string {
	parentClean := filepath.Clean(parentMP)
	var paths []string
	for _, other := range parts {
		otherClean := filepath.Clean(other.MountPoint)
		if otherClean == parentClean {
			continue
		}
		rel, err := filepath.Rel(parentClean, otherClean)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		paths = append(paths, rel)
	}
	return paths
}

// CopyPartition copies content for one partition from the overlay to the
// mounted rootfs, using tar with --exclude for precise nested path exclusion.
func CopyPartition(mergedDir string, part actions.PartitionDef, allParts []actions.PartitionDef, rootfs string) error {
	srcDir := filepath.Join(mergedDir, part.MountPoint)
	destDir := filepath.Join(rootfs, part.MountPoint)

	// Check if there's content to copy
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		fmt.Printf("    %s: (empty)\n", part.MountPoint)
		return nil
	}

	excludes := DescendantMountPaths(part.MountPoint, allParts)

	fmt.Printf("    %s -> %s (%s)\n", part.MountPoint, part.Name, part.Filesystem)

	// Build tar create args
	tarCreate := []string{"-C", srcDir}
	for _, exc := range excludes {
		tarCreate = append(tarCreate, fmt.Sprintf("--exclude=./%s", exc))
	}
	tarCreate = append(tarCreate, "-cf", "-", ".")

	// Build tar extract args
	tarExtract := []string{"-C", destDir}
	switch part.Filesystem {
	case "vfat", "fat32":
		tarExtract = append(tarExtract, "--no-same-owner")
	}
	tarExtract = append(tarExtract, "-xpf", "-")

	if err := runPipe("tar", tarCreate, "tar", tarExtract); err != nil {
		return fmt.Errorf("copying %s: %w", part.MountPoint, err)
	}

	// Propagate the source directory's ownership and mode onto
	// the partition root so permissions set during build phases are preserved.
	// Skip for vfat/fat32 which doesn't support Unix ownership/permissions.
	if part.Filesystem != "vfat" && part.Filesystem != "fat32" {
		if err := preserveDirMeta(srcDir, destDir); err != nil {
			return fmt.Errorf("preserving metadata for %s: %w", part.MountPoint, err)
		}
	}

	return nil
}

// preserveDirMeta copies the ownership and mode from src to dest.
func preserveDirMeta(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := os.Chown(dest, int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}
	return os.Chmod(dest, info.Mode())
}

// copyForFilesystem copies src to dest using the appropriate cp flags
// for the filesystem type. Layer files never inherit host ownership —
// ownership is set by the parent directory in the target rootfs.
func copyForFilesystem(src, dest, filesystem string) error {
	switch filesystem {
	case "vfat", "fat32":
		// vfat: no Unix permissions support at all
		return run("cp", "-rT", "--no-preserve=ownership,mode", src, dest)
	default:
		// Preserve timestamps, symlinks, and file mode bits from the layer source.
		// Ownership is not preserved — files inherit from the parent directory in the target rootfs.
		return run("cp", "-rT", "--no-preserve=ownership", src, dest)
	}
}

// hasPartType returns true if any partition has the given type.
func hasPartType(parts []actions.PartitionDef, partType string) bool {
	for _, p := range parts {
		if p.Type == partType {
			return true
		}
	}
	return false
}

// hasRootPartition returns true if any partition is mounted at "/".
func hasRootPartition(parts []actions.PartitionDef) bool {
	for _, p := range parts {
		if p.MountPoint == "/" {
			return true
		}
	}
	return false
}

// InstallBootloader runs bootctl install in the mounted rootfs if the
// partition layout includes an EFI system partition. At this point /boot
// is a real mounted vfat ESP, so bootctl can detect it properly.
// It also injects root=UUID=<uuid> into boot entries that lack a root= param.
func InstallBootloader(parts []actions.PartitionDef, rootfs string) error {
	if !hasPartType(parts, "efi") {
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render("Installing bootloader"))

	fmt.Printf("    bootctl install\n")
	if err := chrootRun(rootfs, "bootctl", "install"); err != nil {
		return fmt.Errorf("bootctl install: %w", err)
	}

	if !hasRootPartition(parts) {
		return nil
	}

	// Get the filesystem UUID of the root partition from the live mount.
	// At this point the final formatted partition is mounted at rootfs,
	// so findmnt returns the UUID baked in by mkfs — the same UUID the
	// kernel will see at boot via /dev/disk/by-uuid/.
	uuid, err := runOutput("findmnt", "-no", "UUID", rootfs)
	if err != nil || uuid == "" {
		return fmt.Errorf("could not determine root filesystem UUID (is / mounted at %s?)", rootfs)
	}

	fmt.Printf("    root UUID: %s\n", uuid)

	if err := patchBootEntries(rootfs, uuid); err != nil {
		return fmt.Errorf("patching boot entries: %w", err)
	}
	return nil
}

// patchBootEntries adds root=UUID=<uuid> to any systemd-boot entry whose
// options line doesn't already contain a root= parameter.
func patchBootEntries(rootfs, rootUUID string) error {
	entriesDir := filepath.Join(rootfs, "boot", "loader", "entries")
	entries, err := os.ReadDir(entriesDir)
	if err != nil {
		return nil // no entries dir, nothing to patch
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".conf" {
			continue
		}
		path := filepath.Join(entriesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var patched bool
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "options"); ok {
				rest = strings.TrimSpace(rest)
				// Replace any existing root= parameter with the correct UUID
				var opts []string
				for _, opt := range strings.Fields(rest) {
					if !strings.HasPrefix(opt, "root=") {
						opts = append(opts, opt)
					}
				}
				opts = append([]string{fmt.Sprintf("root=UUID=%s", rootUUID)}, opts...)
				line = "options " + strings.Join(opts, " ")
				patched = true
			}
			lines = append(lines, line)
		}

		if patched {
			fmt.Printf("    patched %s\n", entry.Name())
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// GenerateFstab runs genfstab -U against the mounted rootfs to produce
// /etc/fstab with UUID-based entries for all mounted partitions.
// Swap entries from the host are filtered out since genfstab picks up
// system-wide swap mounts that don't belong to the target.
func GenerateFstab(rootfs string) error {
	out, err := runOutput("genfstab", "-U", rootfs)
	if err != nil {
		return fmt.Errorf("genfstab: %w", err)
	}
	out = filterSwapEntries(out)
	return writeFile(filepath.Join(rootfs, "etc", "fstab"), out+"\n")
}

// ConfigureInstallation regenerates fstab and patches boot entries for a
// mounted installation rootfs. Call this after writing partition images to a
// target disk and mounting all partitions under rootfs. The UUIDs are read
// from the live mounted partitions so fstab and boot entries match the actual
// disk, regardless of whether images were dd-copied or freshly formatted.
func ConfigureInstallation(parts []actions.PartitionDef, rootfs string) error {
	if err := GenerateFstab(rootfs); err != nil {
		return fmt.Errorf("generating fstab: %w", err)
	}

	// Re-patch boot entries with the root UUID from the actual disk.
	if hasPartType(parts, "efi") && hasRootPartition(parts) {
		uuid, err := runOutput("findmnt", "-no", "UUID", rootfs)
		if err != nil || uuid == "" {
			return nil // no UUID found, skip patching
		}
		fmt.Printf("    root UUID: %s\n", uuid)
		if err := patchBootEntries(rootfs, uuid); err != nil {
			return fmt.Errorf("patching boot entries: %w", err)
		}
	}

	return nil
}

// filterSwapEntries removes swap mount entries from genfstab output.
// genfstab picks up host swap partitions since swap is system-wide,
// not scoped to the rootfs path.
func filterSwapEntries(fstab string) string {
	var filtered []string
	for _, line := range strings.Split(fstab, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == "swap" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}
