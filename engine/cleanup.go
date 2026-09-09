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
