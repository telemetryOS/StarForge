package engine

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"github.com/telemetryos/starforge/actions"
)

// MountTable tracks mounted partitions so they can be unmounted in reverse order.
type MountTable struct {
	rootfs string
	mounts []mountEntry
}

type mountEntry struct {
	source string
	target string
}

// NewMountTable creates a mount table rooted at the given directory.
func NewMountTable(rootfs string) *MountTable {
	return &MountTable{rootfs: rootfs}
}

// Rootfs returns the root filesystem path.
func (mt *MountTable) Rootfs() string {
	return mt.rootfs
}

// MountAll mounts partitions under the rootfs in the correct order.
// Partitions are sorted by mount point depth so / is mounted first,
// then /boot, /var/log, etc. The source for each partition is provided
// by the caller (image path or device partition path).
func (mt *MountTable) MountAll(parts []PartitionMount) error {
	if err := os.MkdirAll(mt.rootfs, 0o755); err != nil {
		return fmt.Errorf("creating rootfs directory: %w", err)
	}

	// Sort by mount point path length so / comes first, then /boot, /data, then /var/log etc.
	sorted := make([]PartitionMount, len(parts))
	copy(sorted, parts)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].MountPoint) < len(sorted[j].MountPoint)
	})

	for _, pm := range sorted {
		// Partitions without a mount point (e.g. swap) are not mounted.
		if pm.MountPoint == "" {
			continue
		}
		target := filepath.Join(mt.rootfs, pm.MountPoint)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("creating mount point %s: %w", pm.MountPoint, err)
		}

		args := []string{pm.Source, target}
		if pm.Loop {
			args = []string{"-o", "loop", pm.Source, target}
		}

		out.Info("mount %s -> %s", pm.MountPoint, pm.Source)
		if err := run("mount", args...); err != nil {
			return fmt.Errorf("mounting %s: %w", pm.MountPoint, err)
		}

		mt.mounts = append(mt.mounts, mountEntry{
			source: pm.Source,
			target: target,
		})
	}

	return nil
}

// Unmount unmounts all partitions in reverse order.
func (mt *MountTable) Unmount() error {
	var firstErr error
	for i := len(mt.mounts) - 1; i >= 0; i-- {
		entry := mt.mounts[i]
		out.Info("umount %s", entry.target)
		if err := run("umount", "-R", entry.target); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("unmounting %s: %w", entry.target, err)
			}
		}
	}
	mt.mounts = nil
	return firstErr
}

// PartitionMount describes a partition ready to be mounted.
type PartitionMount struct {
	Source     string // image file path or device partition path
	MountPoint string // e.g. "/", "/boot"
	Loop       bool   // true for image files (need -o loop)
}

// blockDevSize returns the size in bytes of a block device using the
// BLKGETSIZE64 ioctl. This replaces "blockdev --getsize64 <device>".
func blockDevSize(device string) (uint64, error) {
	f, err := os.Open(device)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	const blkgetsize64 = 0x80081272 // ioctl(fd, BLKGETSIZE64, &uint64)
	var sz uint64
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), blkgetsize64, uintptr(unsafe.Pointer(&sz))); errno != 0 {
		return 0, fmt.Errorf("BLKGETSIZE64 %s: %w", device, errno)
	}
	return sz, nil
}

// resolveBin looks for a binary in the vendored bin dir first,
// then falls back to the system PATH.
func resolveBin(name string) string {
	vendored := filepath.Join(VendorBinDir(), name)
	if _, err := os.Stat(vendored); err == nil {
		return vendored
	}
	return name
}

// run executes a command with visible output routed through the output system.
// It checks the vendored bin directory first for the binary and
// sets PATH/LD_LIBRARY_PATH so child processes also find vendored tools.
func run(name string, args ...string) error {
	cmd := exec.Command(resolveBin(name), args...)
	cmd.Env = vendorEnv()
	if out != nil {
		cmd.Stdout = out.ProcessWriter()
		cmd.Stderr = out.ProcessWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// runSilent executes a command with output suppressed from the terminal.
// Output goes to the log file only. Used for noisy subprocesses like mkfs,
// tar, zstd, and go build.
func runSilent(name string, args ...string) error {
	cmd := exec.Command(resolveBin(name), args...)
	cmd.Env = vendorEnv()
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// runPipeSilent connects stdout of cmd1 to stdin of cmd2 with output
// suppressed from the terminal. Stderr goes to the log file only.
func runPipeSilent(cmd1Name string, cmd1Args []string, cmd2Name string, cmd2Args []string) error {
	c1 := exec.Command(resolveBin(cmd1Name), cmd1Args...)
	c2 := exec.Command(resolveBin(cmd2Name), cmd2Args...)
	c1.Env = vendorEnv()
	c2.Env = vendorEnv()
	if out != nil {
		c1.Stderr = out.LogWriter()
		c2.Stderr = out.LogWriter()
	} else {
		c1.Stderr = os.Stderr
		c2.Stderr = os.Stderr
	}

	pipe, err := c1.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}
	c2.Stdin = pipe

	if err := c1.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", cmd1Name, err)
	}
	if err := c2.Start(); err != nil {
		c1.Process.Kill()
		c1.Wait()
		return fmt.Errorf("starting %s: %w", cmd2Name, err)
	}

	err2 := c2.Wait()
	err1 := c1.Wait()
	if err1 != nil {
		return fmt.Errorf("%s: %w", cmd1Name, err1)
	}
	if err2 != nil {
		return fmt.Errorf("%s: %w", cmd2Name, err2)
	}
	return nil
}

// runOutput executes a command and returns its stdout.
func runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(resolveBin(name), args...)
	cmd.Env = vendorEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// vendorEnv returns the current environment with vendored paths prepended
// to PATH and LD_LIBRARY_PATH so vendored binaries and libraries are found.
func vendorEnv() []string {
	binDir := VendorBinDir()
	libDir := VendorLibDir()
	env := os.Environ()

	hasPath, hasLdPath := false, false
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = fmt.Sprintf("PATH=%s:%s", binDir, e[5:])
			hasPath = true
		}
		if strings.HasPrefix(e, "LD_LIBRARY_PATH=") {
			env[i] = fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libDir, e[16:])
			hasLdPath = true
		}
	}

	if !hasPath {
		env = append(env, fmt.Sprintf("PATH=%s", binDir))
	}
	if !hasLdPath {
		env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s", libDir))
	}

	return env
}

// SetupImagePartitions creates sparse image files, formats them, and returns
// PartitionMounts ready for MountAll. This is the default build mode.
func SetupImagePartitions(parts []actions.PartitionDef, buildDir string) ([]PartitionMount, error) {
	var mounts []PartitionMount

	for _, part := range parts {
		// Partitions with Size=0 are percentage-growable (e.g. 100%) and have
		// no defined minimum size. They cannot be formatted as image files
		// because there is no target disk to resolve the size against.
		// Use a fixed minimum size (e.g. "7G+") for image-based builds.
		if part.Size == 0 {
			return nil, fmt.Errorf(
				"partition %q has no fixed size (use a minimum size like \"7G+\" for image builds; 100%%-only partitions require direct device installation)",
				part.Name)
		}

		imgPath := filepath.Join(buildDir, fmt.Sprintf("%s.img", part.Name))
		sizeLabel := actions.FormatSize(part.Size)

		out.Info("%s.img (%s, %s)", part.Name, sizeLabel, part.Filesystem)

		// Create sparse image file
		f, err := os.Create(imgPath)
		if err != nil {
			return nil, fmt.Errorf("creating image %s: %w", part.Name, err)
		}
		if err := f.Truncate(int64(part.Size)); err != nil {
			f.Close()
			return nil, fmt.Errorf("setting image size %s: %w", part.Name, err)
		}
		f.Close()

		// Format the image
		if err := formatFilesystem(imgPath, part.Filesystem, part.Name); err != nil {
			return nil, err
		}

		mounts = append(mounts, PartitionMount{
			Source:     imgPath,
			MountPoint: part.MountPoint,
			Loop:       true,
		})
	}

	return mounts, nil
}

// PartitionDevice partitions a block device with GPT using sfdisk and returns
// the resolved partition list (with grow sizes applied). It does NOT format
// the partitions — the caller decides what to do with each one.
func PartitionDevice(parts []actions.PartitionDef, device string) ([]actions.PartitionDef, error) {
	out.Info("partitioning %s", device)

	// Get device size for resolving growable partitions
	deviceSize, err := blockDevSize(device)
	if err != nil {
		return nil, fmt.Errorf("getting device size: %w", err)
	}

	const gptOverhead = 2 * 1024 * 1024
	usableSize := deviceSize
	if usableSize > gptOverhead {
		usableSize -= gptOverhead
	}

	resolved := ResolvePartitionSizes(parts, usableSize)

	var script strings.Builder
	script.WriteString("label: gpt\n")
	for _, part := range resolved {
		sizeSectors := part.Size / 512
		sizeLabel := actions.FormatSize(part.Size)
		out.Info("partition: %s (%s, %s)", part.Name, sizeLabel, part.Filesystem)
		fmt.Fprintf(&script, "size=%d, type=%s, name=%q\n",
			sizeSectors, sfdiskTypeAlias(part.Type), part.Name)
	}

	cmd := exec.Command(resolveBin("sfdisk"), "--force", device)
	cmd.Env = vendorEnv()
	cmd.Stdin = strings.NewReader(script.String())
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("partitioning %s: %w", device, err)
	}

	run("partprobe", device)

	return resolved, nil
}

// PartitionPath returns the device path for a numbered partition.
// Handles /dev/sdX, /dev/nvmeXnY, /dev/loopN, and /dev/mmcblkN naming.
func PartitionPath(device string, num int) string {
	return partitionPath(device, num)
}

// SetupDevicePartitions partitions a block device with GPT using sfdisk,
// formats each partition, and returns PartitionMounts ready for MountAll.
func SetupDevicePartitions(parts []actions.PartitionDef, device string) ([]PartitionMount, error) {
	resolved, err := PartitionDevice(parts, device)
	if err != nil {
		return nil, err
	}

	// Format and collect mounts
	var mounts []PartitionMount
	for i, part := range resolved {
		partPath := partitionPath(device, i+1)

		if err := formatFilesystem(partPath, part.Filesystem, part.Name); err != nil {
			return nil, err
		}

		mounts = append(mounts, PartitionMount{
			Source:     partPath,
			MountPoint: part.MountPoint,
			Loop:       false,
		})
	}

	return mounts, nil
}

// formatFilesystem formats a partition or image file with the specified filesystem.
func formatFilesystem(path, filesystem, name string) error {
	return out.RunWithSpinner(fmt.Sprintf("format %s (%s)", name, filesystem), func() error {
		switch filesystem {
		case "vfat", "fat32":
			if err := runSilent("mkfs.vfat", "-F", "32", path); err != nil {
				return fmt.Errorf("formatting %s as vfat: %w", name, err)
			}
		case "ext4":
			if err := runSilent("mkfs.ext4", "-F", "-L", name, path); err != nil {
				return fmt.Errorf("formatting %s as ext4: %w", name, err)
			}
		case "btrfs":
			if err := runSilent("mkfs.btrfs", "-f", "-L", name, path); err != nil {
				return fmt.Errorf("formatting %s as btrfs: %w", name, err)
			}
		case "xfs":
			if err := runSilent("mkfs.xfs", "-f", "-L", name, path); err != nil {
				return fmt.Errorf("formatting %s as xfs: %w", name, err)
			}
		case "f2fs":
			if err := runSilent("mkfs.f2fs", "-l", name, path); err != nil {
				return fmt.Errorf("formatting %s as f2fs: %w", name, err)
			}
		case "swap":
			if err := runSilent("mkswap", "-L", name, path); err != nil {
				return fmt.Errorf("formatting %s as swap: %w", name, err)
			}
		default:
			return fmt.Errorf("unsupported filesystem: %s", filesystem)
		}
		return nil
	})
}

// partitionPath returns the device path for a numbered partition.
// Handles both /dev/sdX and /dev/nvmeXnY naming conventions.
func partitionPath(device string, num int) string {
	base := filepath.Base(device)
	if strings.HasPrefix(base, "nvme") || strings.HasPrefix(base, "loop") || strings.HasPrefix(base, "mmcblk") {
		return fmt.Sprintf("%sp%d", device, num)
	}
	return fmt.Sprintf("%s%d", device, num)
}
