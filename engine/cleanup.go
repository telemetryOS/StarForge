package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CleanupMounts unmounts any leftover mounts under scope.
// Does NOT touch device mappers or loops. Safe to call when
// other mounts in the parent directory must be preserved.
func CleanupMounts(scope string) {
	absDir, err := filepath.Abs(scope)
	if err != nil {
		return
	}

	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return
	}

	var mounts []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mountPoint := fields[1]
		if mountPoint == absDir || strings.HasPrefix(mountPoint, absDir+"/") {
			mounts = append(mounts, mountPoint)
		}
	}

	if len(mounts) == 0 {
		return
	}

	// Unmount deepest paths first
	sort.Slice(mounts, func(i, j int) bool {
		return len(mounts[i]) > len(mounts[j])
	})

	if out != nil {
		out.Info("cleaning up stale mounts")
	} else {
		fmt.Println("  Cleaning up stale mounts")
	}
	for _, mp := range mounts {
		if out != nil {
			out.SubInfo("umount %s", mp)
		} else {
			fmt.Printf("    umount %s\n", mp)
		}
		run("umount", "-R", mp)
	}
	if out != nil {
		out.Blank()
	} else {
		fmt.Println()
	}
}

// CleanupAll cleans up ALL stale resources: mounts under scope,
// starforge-* device mappers, and loop devices backed by files in scope.
// Use at CLI entry points (clean, export, write) where any resource
// from a previous crashed operation could be stale.
func CleanupAll(scope string) {
	absDir, err := filepath.Abs(scope)
	if err != nil {
		absDir = scope
	}

	CleanupMounts(scope)
	cleanupDeviceMappers()
	cleanupLoops(absDir)
}

// cleanupDeviceMappers removes any starforge-* device mapper devices
// left over from a previous QEMU run. Device mapper devices hold loop devices
// open, so they must be removed before loop cleanup can succeed.
func cleanupDeviceMappers() {
	entries, err := filepath.Glob("/dev/mapper/starforge-*")
	if err != nil || len(entries) == 0 {
		return
	}

	for _, entry := range entries {
		dmName := filepath.Base(entry)

		// Skip partition sub-devices (e.g. starforge-device-abcd1234p1)
		// — they are removed by cleanupDeviceMapper
		if strings.Contains(dmName, "p") {
			parts := strings.SplitN(dmName, "p", -1)
			lastPart := parts[len(parts)-1]
			if _, err := fmt.Sscanf(lastPart, "%d", new(int)); err == nil {
				continue
			}
		}

		if out != nil {
			out.Info("cleaning up stale device mapper: %s", dmName)
		} else {
			fmt.Printf("  Cleaning up stale device mapper: %s\n", dmName)
		}
		cleanupStaleDeviceMapper(dmName)
	}
}

// cleanupStaleDeviceMapper checks for and cleans up a device mapper device
// left over from a previous crashed run.
func cleanupStaleDeviceMapper(dmName string) {
	// Check if the dm device exists
	dmPath := filepath.Join("/dev/mapper", dmName)
	if _, err := os.Stat(dmPath); os.IsNotExist(err) {
		return
	}

	// Parse dm table to find loop devices
	table, err := runOutput("dmsetup", "table", dmName)
	if err != nil {
		// Device exists but can't read table; try force remove
		run("dmsetup", "remove", "--force", dmName)
		return
	}

	// Extract loop device paths from the table (format: "start len linear /dev/loopN offset")
	var loopDevs []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		for _, f := range fields {
			if strings.HasPrefix(f, "/dev/loop") && !seen[f] {
				seen[f] = true
				loopDevs = append(loopDevs, f)
			}
		}
	}

	cleanupDeviceMapper(dmName, loopDevs)
}

// cleanupLoops detaches any loop devices whose backing file is inside dir.
// It first unmounts any filesystems mounted from those loop devices, then
// detaches the loop devices themselves.
func cleanupLoops(dir string) {
	loopList, err := runOutput("losetup", "-l", "-n", "-O", "NAME,BACK-FILE")
	if err != nil {
		return
	}

	// Collect stale loop devices
	var staleLoops []string
	for _, line := range strings.Split(loopList, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		loopDev := fields[0]
		backFile := strings.Join(fields[1:], " ")
		backFile = strings.TrimSuffix(backFile, " (deleted)")
		if strings.HasPrefix(backFile, dir+"/") {
			staleLoops = append(staleLoops, loopDev)
		}
	}

	if len(staleLoops) == 0 {
		return
	}

	// Unmount any filesystems mounted from the stale loop devices
	mountData, err := os.ReadFile("/proc/mounts")
	if err == nil {
		staleSet := make(map[string]bool, len(staleLoops))
		for _, dev := range staleLoops {
			staleSet[dev] = true
		}
		for _, line := range strings.Split(string(mountData), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			if staleSet[fields[0]] {
				if out != nil {
				out.SubInfo("umount stale loop: %s", fields[1])
			} else {
				fmt.Printf("  Unmounting stale loop mount: %s\n", fields[1])
			}
				run("umount", fields[1])
			}
		}
	}

	// Detach the loop devices
	for _, loopDev := range staleLoops {
		if out != nil {
			out.SubInfo("detach stale loop: %s", loopDev)
		} else {
			fmt.Printf("  Detaching stale loop: %s\n", loopDev)
		}
		run("losetup", "-d", loopDev)
	}
}

