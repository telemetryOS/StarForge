package engine

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

// PackageOps holds file ownership and permission operations to apply on
// the packaged images via chroot. Usernames are resolved from the target's
// /etc/passwd so UIDs match the target system.
type PackageOps struct {
	Ownerships  []actions.FileOwnershipOp
	Permissions []actions.FilePermissionOp
}

// PackageToImages creates sparse partition images, formats them, and copies
// the merged overlay tree into the appropriate partitions.
func PackageToImages(mergedDir string, parts []actions.PartitionDef, buildDir string, ops PackageOps) error {
	out.Blank()
	out.Phase("Packaging images")

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

	if err := packagePipeline(mergedDir, parts, mounts, rootfs, "images", ops); err != nil {
		return err
	}

	// Save partition layout so non-build commands (run, export) can read
	// it without re-running Collect.
	return SavePartitions(parts, buildDir)
}

// WriteToDevice partitions a block device and writes pre-built partition
// images using bmaptool so sparse image extents are skipped.
// Growable ext4 partitions are expanded to fill the device partition.
// This replaces PackageToDevice — no overlay mount or tar copy is needed
// because the images already contain the complete filesystem.
func WriteToDevice(parts []actions.PartitionDef, device, buildDir string) error {
	out.StartStage(StageWrite)
	writeStart := time.Now()

	out.Blank()
	out.Phase("Writing to device")

	// Partition the device (GPT via sfdisk)
	resolved, err := PartitionDevice(parts, device)
	if err != nil {
		return fmt.Errorf("partitioning device: %w", err)
	}

	// Write each partition image to the device
	for i, part := range resolved {
		partDev := partitionPath(device, i+1)
		imgPath := filepath.Join(buildDir, fmt.Sprintf("%s.img", part.Name))

		bmapPath := imgPath + ".bmap"
		if err := ensureBmap(imgPath, bmapPath); err != nil {
			return fmt.Errorf("creating bmap for %s: %w", part.Name, err)
		}

		if err := out.RunWithProgress(fmt.Sprintf("bmaptool %s -> %s", part.Name, partDev), func(update func(int)) error {
			return runBmaptoolCopy(imgPath, bmapPath, partDev, update)
		}); err != nil {
			return fmt.Errorf("writing %s: %w", part.Name, err)
		}

		// Expand filesystem if the device partition is larger than the image
		if resolved[i].Size > parts[i].Size {
			out.Info("resize %s (%s -> %s)", part.Name,
				actions.FormatSize(parts[i].Size), actions.FormatSize(resolved[i].Size))
			if err := expandFilesystem(partDev, part.Filesystem); err != nil {
				return fmt.Errorf("expanding %s: %w", part.Name, err)
			}
		}
	}

	out.EndStage(StageWrite, time.Since(writeStart))
	return nil
}

// EnsureBmap creates or refreshes the bmap sidecar for imagePath.
func EnsureBmap(imagePath, bmapPath string) error {
	imgInfo, err := os.Stat(imagePath)
	if err != nil {
		return err
	}
	bmapInfo, err := os.Stat(bmapPath)
	if err == nil && !bmapInfo.ModTime().Before(imgInfo.ModTime()) {
		return nil
	}
	return runSilent("bmaptool", "create", "-o", bmapPath, imagePath)
}

// ensureBmap is kept for package-local callers that predate the exported
// helper name.
func ensureBmap(imagePath, bmapPath string) error {
	return EnsureBmap(imagePath, bmapPath)
}

func runBmaptoolCopy(imagePath, bmapPath, destPath string, update func(int)) error {
	tmpDir, err := os.MkdirTemp("", "starforge-bmap-progress-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	progressPath := filepath.Join(tmpDir, "progress")
	if err := syscall.Mkfifo(progressPath, 0o600); err != nil {
		return fmt.Errorf("creating progress fifo: %w", err)
	}
	progressFile, err := os.OpenFile(progressPath, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("opening progress fifo: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(progressFile)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) == 2 && fields[0] == "PROGRESS" {
				if pct, err := strconv.Atoi(fields[1]); err == nil {
					update(pct)
				}
			}
		}
	}()

	cmd := exec.Command(resolveBin("bmaptool"), "copy", "--bmap", bmapPath, "--psplash-pipe", progressPath, imagePath, destPath)
	cmd.Env = vendorEnv()
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	err = cmd.Run()
	progressFile.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	if err == nil {
		update(100)
	}
	return err
}

// WriteToDiskImage creates a sparse disk image from pre-built partition images.
// It calculates the minimum disk size from the partition layout, creates a
// sparse file, attaches it as a loop device, and writes partition images using
// the same WriteToDevice flow used for block devices.
//
// Returns the loop device path for additional operations (e.g. installer
// bundling) and a cleanup function that detaches the loop device. The cleanup
// function is idempotent and safe to call multiple times.
func WriteToDiskImage(parts []actions.PartitionDef, buildDir, imagePath string) (loopDev string, cleanup func(), err error) {
	out.Blank()
	out.Phase("Creating disk image")

	// Calculate minimum disk size: sum of partition sizes + GPT overhead
	var totalSize uint64
	for _, p := range parts {
		totalSize += p.Size
	}
	totalSize += 2 * 1024 * 1024 // GPT overhead (1MB front + 1MB back)

	// Round up to nearest MiB for alignment
	const mib = 1024 * 1024
	totalSize = ((totalSize + mib - 1) / mib) * mib

	sizeLabel := actions.FormatSize(totalSize)
	out.Info("%s (%s)", imagePath, sizeLabel)

	// Create sparse file
	f, err := os.Create(imagePath)
	if err != nil {
		return "", nil, fmt.Errorf("creating disk image: %w", err)
	}
	if err := f.Truncate(int64(totalSize)); err != nil {
		f.Close()
		return "", nil, fmt.Errorf("setting disk image size: %w", err)
	}
	f.Close()

	// Attach loop device with partition scanning enabled
	loopDev, err = runOutput("losetup", "--find", "--show", "--partscan", imagePath)
	if err != nil {
		os.Remove(imagePath)
		return "", nil, fmt.Errorf("attaching loop device: %w", err)
	}
	out.Info("loop device: %s", loopDev)

	var detached bool
	cleanup = func() {
		if !detached {
			detached = true
			out.Info("detaching %s", loopDev)
			run("losetup", "-d", loopDev)
		}
	}

	// Write partition images to the loop device
	if err := WriteToDevice(parts, loopDev, buildDir); err != nil {
		cleanup()
		os.Remove(imagePath)
		return "", nil, fmt.Errorf("writing to disk image: %w", err)
	}

	return loopDev, cleanup, nil
}

// CompressDiskImage compresses a raw disk image with gzip for compatibility
// with flash tools (Balena Etcher, Rufus). Returns the path to the compressed
// file. The original uncompressed file is removed on success.
func CompressDiskImage(imagePath string) (string, error) {
	out.Blank()
	out.Phase("Compressing disk image")

	gzPath := imagePath + ".gz"

	if err := out.RunWithSpinner(fmt.Sprintf("gzip %s", filepath.Base(gzPath)), func() error {
		return gzipFile(imagePath, gzPath)
	}); err != nil {
		return "", fmt.Errorf("compressing disk image: %w", err)
	}

	return gzPath, nil
}

// gzipFile compresses src into dest using gzip best-compression and removes src on success.
func gzipFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	outFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gw, err := gzip.NewWriterLevel(outFile, gzip.BestCompression)
	if err != nil {
		outFile.Close()
		os.Remove(dest)
		return err
	}

	if _, err := io.Copy(gw, in); err != nil {
		gw.Close()
		outFile.Close()
		os.Remove(dest)
		return err
	}
	if err := gw.Close(); err != nil {
		outFile.Close()
		os.Remove(dest)
		return err
	}
	if err := outFile.Close(); err != nil {
		os.Remove(dest)
		return err
	}

	in.Close()
	return os.Remove(src)
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
func PackageToDiskImage(mergedDir string, parts []actions.PartitionDef, diskSize uint64, outputPath string, ops PackageOps) error {
	out.Blank()
	out.Phase("Creating disk image")

	// Create sparse file
	sizeLabel := actions.FormatSize(diskSize)
	out.Info("%s (%s)", outputPath, sizeLabel)

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
	out.Info("loop device: %s", loopDev)

	// Detach loop device on exit
	defer func() {
		out.Info("detaching %s", loopDev)
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

	return packagePipeline(mergedDir, parts, mounts, rootfs, "disk image", ops)
}

// packagePipeline is the common pipeline shared by all Package functions.
// It verifies the overlay, mounts partitions, copies content, applies
// ownership/permissions via chroot, installs the bootloader, generates
// fstab, and unmounts.
func packagePipeline(mergedDir string, parts []actions.PartitionDef, mounts []PartitionMount, rootfs, label string, ops PackageOps) error {
	// Verify overlay is mounted and populated before we start
	if err := verifyOverlay(mergedDir); err != nil {
		return fmt.Errorf("source overlay: %w", err)
	}

	// Mount partitions
	mt := NewMountTable(rootfs)

	out.Blank()
	out.Phase(fmt.Sprintf("Mounting %s", label))
	if err := mt.MountAll(mounts); err != nil {
		mt.Unmount()
		return fmt.Errorf("mounting %s: %w", label, err)
	}
	defer func() {
		out.Blank()
		out.Phase(fmt.Sprintf("Unmounting %s", label))
		mt.Unmount()
	}()

	// Copy merged tree into partitions
	out.Blank()
	out.Phase(fmt.Sprintf("Copying to %s", label))
	if err := copyToPartitions(mergedDir, parts, rootfs); err != nil {
		return err
	}

	// Ensure chroot directories exist for bootloader and ownership fixup
	if err := EnsureChrootDirs(rootfs); err != nil {
		return err
	}

	// Install bootloader now that the ESP is a real mounted vfat. This is
	// the legacy single-target path (PackageToImages); the multi-target
	// flow in PackageMultiTarget runs InstallSystemdBoot + PatchBootEntries
	// per-target in packageOneTarget instead.
	if err := InstallSystemdBoot(parts, rootfs); err != nil {
		return err
	}

	// Generate /etc/fstab with UUIDs from the formatted partitions.
	// This must run before applyImageOwnership because arch-chroot
	// bind-mounts /proc, /sys, /dev into the rootfs which can pollute
	// the mount table that genfstab reads.
	out.Blank()
	out.Phase("Generating fstab")
	if err := GenerateFstab(rootfs); err != nil {
		return err
	}

	// Apply file ownership and permissions on the real partition images.
	// This runs inside the chroot so usernames are resolved from the
	// target's /etc/passwd — not from overlay metadata which may have
	// been corrupted by ChownToInvoker on cached builds.
	if err := applyImageOwnership(rootfs, ops); err != nil {
		return err
	}

	return nil
}

// applyImageOwnership applies file ownership and permission operations on
// the mounted partition images via chroot. Usernames are resolved from
// the target's /etc/passwd, ensuring correct UIDs regardless of overlay
// cache state.
func applyImageOwnership(rootfs string, ops PackageOps) error {
	if len(ops.Ownerships) == 0 && len(ops.Permissions) == 0 {
		return nil
	}

	out.Blank()
	out.Phase("Applying ownership & permissions")

	for _, own := range ops.Ownerships {
		spec := own.Owner + ":" + own.Group
		out.Info("chown %s %s%s", spec, own.Path, labelSuffix(own.Label))

		args := []string{"chown"}
		if own.Recursive {
			args = append(args, "-R")
		}
		args = append(args, spec, own.Path)
		if err := ChrootRun(rootfs, args...); err != nil {
			return fmt.Errorf("chown %s: %w", own.Path, err)
		}
	}

	for _, perm := range ops.Permissions {
		out.Info("chmod %s %s%s", perm.Mode, perm.Path, labelSuffix(perm.Label))

		args := []string{"chmod"}
		if perm.Recursive {
			args = append(args, "-R")
		}
		args = append(args, perm.Mode, perm.Path)
		if err := ChrootRun(rootfs, args...); err != nil {
			return fmt.Errorf("chmod %s: %w", perm.Path, err)
		}
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
//
// In multi-target builds this is called once per target; a later target's
// extraction overwrites an earlier target's files at the same path. The
// engine does not arbitrate; conflicts at shared paths must be designed
// out at the layer level.
func CopyPartition(mergedDir string, part actions.PartitionDef, allParts []actions.PartitionDef, rootfs string) error {
	srcDir := filepath.Join(mergedDir, part.MountPoint)
	destDir := filepath.Join(rootfs, part.MountPoint)

	// Check if there's content to copy
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		out.Info("%s: (empty)", part.MountPoint)
		return nil
	}

	excludes := DescendantMountPaths(part.MountPoint, allParts)

	copyLabel := fmt.Sprintf("%s -> %s (%s)", part.MountPoint, part.Name, part.Filesystem)

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

	if err := out.RunWithSpinner(copyLabel, func() error {
		return runPipeSilent("tar", tarCreate, "tar", tarExtract)
	}); err != nil {
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

// copyForFilesystem copies src to dest for the given filesystem type.
// vfat does not support Unix permissions, so mode bits are not preserved.
// For other filesystems, mode bits are preserved but ownership is not —
// ownership is set by the parent directory in the target rootfs.
func copyForFilesystem(src, dest, filesystem string) error {
	switch filesystem {
	case "vfat", "fat32":
		return copyTree(src, dest, false)
	default:
		return copyTree(src, dest, true)
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

// InstallSystemdBoot runs `bootctl install` against the mounted rootfs's
// chroot if the partition layout includes an EFI system partition. This
// writes the systemd-boot binary to the ESP and is meant to run ONCE per
// disk — only the host of a multi-target build invokes it.
func InstallSystemdBoot(parts []actions.PartitionDef, rootfs string) error {
	if !hasPartType(parts, "efi") {
		return nil
	}

	out.Blank()
	out.Phase("Installing bootloader")
	out.Info("bootctl install")
	if err := ChrootRun(rootfs, "bootctl", "install"); err != nil {
		return fmt.Errorf("bootctl install: %w", err)
	}
	return nil
}

// PatchBootEntries injects `root=UUID=<this-rootfs-/-uuid>` into each
// entry's options. Each target invokes this against its own packageOneTarget
// rootfs during its own pass, so per-target isolation gives every entry the
// correct root UUID — recovery's entries get the recovery partition's UUID,
// fallback-recovery's get the fallback-recovery UUID, the host gets its own
// root partition's UUID. No cross-target inference.
//
// Walks both the ESP (`<rootfs><efi-mount>/loader/entries/`) and XBOOTLDR
// (`<rootfs><xbootldr-mount>/loader/entries/`) entries directories based on
// each entry's `extended` flag, resolved the same way phase_boot resolves
// it for writing.
func PatchBootEntries(parts []actions.PartitionDef, entries []config.BootEntry, rootfs string) error {
	if len(entries) == 0 {
		return nil
	}
	if !hasRootPartition(parts) {
		return nil
	}

	uuid, err := runOutput("findmnt", "-no", "UUID", rootfs)
	if err != nil || uuid == "" {
		return fmt.Errorf("could not determine root filesystem UUID (is / mounted at %s?)", rootfs)
	}

	// Build a synthetic build context so we can reuse the existing
	// resolveExtended/findPartitionByType helpers from phase_boot.
	synth := &actions.BuildContext{Partitions: parts}

	for _, entry := range entries {
		ext := resolveExtended(synth, entry.Extended)
		var partType string
		if ext {
			partType = "xbootldr"
		} else {
			partType = "efi"
		}
		part, ok := findPartitionByType(synth, partType)
		if !ok {
			return fmt.Errorf("entry %q: no partition with type %q declared by this target", entry.Name, partType)
		}

		name := entry.Name
		if !strings.HasSuffix(name, ".conf") {
			name += ".conf"
		}
		confPath := filepath.Join(rootfs, part.MountPoint, "loader/entries", name)

		if err := injectRootUUID(confPath, uuid); err != nil {
			return fmt.Errorf("patching %s: %w", name, err)
		}
		out.Info("patched %s -> root=UUID=%s", name, uuid)
	}
	return nil
}

// injectRootUUID rewrites the `options` line of a systemd-boot entry .conf,
// replacing any existing `root=` token with `root=UUID=<rootUUID>`.
func injectRootUUID(path, rootUUID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lines []string
	patched := false
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "options"); ok {
			rest = strings.TrimSpace(rest)
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
	if !patched {
		return fmt.Errorf("entry has no options line to patch")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
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

// ConfigureInstallation regenerates fstab for a mounted installation rootfs.
// Call this after writing partition images to a target disk and mounting all
// partitions under rootfs.
//
// Boot-entry root=UUID values are baked in at build time and travel through
// dd-copy unchanged, so no runtime re-patching is needed here.
func ConfigureInstallation(parts []actions.PartitionDef, rootfs string) error {
	if err := GenerateFstab(rootfs); err != nil {
		return fmt.Errorf("generating fstab: %w", err)
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

// SavePartitions writes the partition layout to partitions.json in buildDir.
// Non-build commands (run, export) read this instead of re-running Collect.
func SavePartitions(parts []actions.PartitionDef, buildDir string) error {
	data, err := json.MarshalIndent(parts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling partitions: %w", err)
	}
	return os.WriteFile(filepath.Join(buildDir, "partitions.json"), data, 0o644)
}

// BuildResult captures the subset of BuildContext that packaging needs.
// Saved by Build so EnsurePackaged can avoid re-running Collect.
type BuildResult struct {
	Partitions      []actions.PartitionDef      `json:"partitions"`
	Ownerships      []actions.FileOwnershipOp   `json:"ownerships,omitempty"`
	Permissions     []actions.FilePermissionOp  `json:"permissions,omitempty"`
	InstallPayloads []actions.InstallPayloadDef `json:"install_payloads,omitempty"`
	InstallServer   *actions.InstallServerDef   `json:"install_server,omitempty"`
	InstallClient   *actions.InstallClientDef   `json:"install_client,omitempty"`
	InstallEmbeds   []string                    `json:"install_embeds,omitempty"`
	Boot            *actions.BootConfig         `json:"boot,omitempty"`
}

// SaveBuildResult writes the packaging-relevant context to build-result.json.
// Mapping is shared with HashPackaging via contextToBuildResult so the two
// stay in lockstep — adding a field to BuildResult and contextToBuildResult
// gets you persistence, cache invalidation, and the rehydrated BuildContext
// for free.
func SaveBuildResult(ctx *actions.BuildContext, buildDir string) error {
	r := contextToBuildResult(ctx)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling build result: %w", err)
	}
	return os.WriteFile(filepath.Join(buildDir, "build-result.json"), data, 0o644)
}

// buildResultToContext converts a saved BuildResult into a BuildContext
// with all fields populated (partitions, installer defs, ownership ops).
func buildResultToContext(r *BuildResult) *actions.BuildContext {
	return &actions.BuildContext{
		Partitions:      r.Partitions,
		FileOwnerships:  r.Ownerships,
		FilePermissions: r.Permissions,
		InstallPayloads: r.InstallPayloads,
		InstallServer:   r.InstallServer,
		InstallClient:   r.InstallClient,
		InstallEmbeds:   r.InstallEmbeds,
		Boot:            r.Boot,
	}
}

// LoadBuildResult reads the packaging context saved by a previous build.
func LoadBuildResult(buildDir string) (*BuildResult, error) {
	data, err := os.ReadFile(filepath.Join(buildDir, "build-result.json"))
	if err != nil {
		return nil, fmt.Errorf("reading build-result.json: %w", err)
	}
	var r BuildResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing build-result.json: %w", err)
	}
	return &r, nil
}
