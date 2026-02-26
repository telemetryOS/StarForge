package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var cleanCmd = &cobra.Command{
	Use:   "clean <target> [scope]",
	Short: "Remove build artifacts",
	Long: `Remove build artifacts for a target or vendored dependencies.

  starforge clean <target>            Remove all build artifacts for a target
  starforge clean <target> cache      Remove only the overlay cache
  starforge clean <target> images     Remove only partition images
  starforge clean <target> disks      Remove extra QEMU disks
  starforge clean deps                Remove vendored dependencies (~/.local/share/starforge/)
  starforge clean pacman              Remove persistent pacman package cache (~/.local/state/starforge/pacman/)`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
	subject := args[0]

	// Special cases: global clean targets (not per-target)
	switch subject {
	case "deps":
		return cleanDeps()
	case "pacman":
		return cleanPacmanCache()
	}

	// Otherwise it's a target name
	targetName := subject

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	if _, ok := proj.Targets[targetName]; !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	scope := ""
	if len(args) > 1 {
		scope = args[1]
	}

	buildDir := proj.TargetBuildDir(targetName)

	switch scope {
	case "":
		return cleanTarget(buildDir, targetName)
	case "cache":
		return cleanCache(buildDir, targetName)
	case "images":
		return cleanImages(buildDir, targetName)
	case "disks":
		return cleanDisks(buildDir, targetName)
	default:
		return fmt.Errorf("unknown scope %q — must be 'cache', 'images', or 'disks'", scope)
	}
}

func cleanTarget(buildDir, targetName string) error {
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("Nothing to clean for target %q", targetName)))
		return nil
	}

	// Build cache contains root-owned files; elevate if needed
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	// Clean up any stale mounts before removing
	engine.CleanupAll(buildDir)

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", buildDir)))
	if err := os.RemoveAll(buildDir); err != nil {
		return fmt.Errorf("removing build directory: %w", err)
	}
	return nil
}

func cleanCache(buildDir, targetName string) error {
	cacheDir := filepath.Join(buildDir, "cache")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("No cache to clean for target %q", targetName)))
		return nil
	}

	// Build cache contains root-owned files; elevate if needed
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}

	// Clean up any stale mounts before removing
	engine.CleanupAll(buildDir)

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", cacheDir)))
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("removing cache: %w", err)
	}
	return nil
}

func cleanImages(buildDir, targetName string) error {
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("No images to clean for target %q", targetName)))
		return nil
	}

	// Remove *.img files and rootfs/
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return fmt.Errorf("reading build directory: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(buildDir, name)

		if filepath.Ext(name) == ".img" || name == "rootfs" {
			fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", path)))
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("removing %s: %w", name, err)
			}
			removed++
		}
	}

	if removed == 0 {
		fmt.Println(cmdDim.Render(fmt.Sprintf("No images to clean for target %q", targetName)))
	}
	return nil
}

func cleanDisks(buildDir, targetName string) error {
	diskDir := filepath.Join(buildDir, "disks")
	if _, err := os.Stat(diskDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("No disks to clean for target %q", targetName)))
		return nil
	}

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", diskDir)))
	if err := os.RemoveAll(diskDir); err != nil {
		return fmt.Errorf("removing disks: %w", err)
	}
	return nil
}

func cleanDeps() error {
	vendorDir := engine.VendorDir()
	if _, err := os.Stat(vendorDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render("No vendored dependencies to clean"))
		return nil
	}

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", vendorDir)))
	if err := os.RemoveAll(vendorDir); err != nil {
		return fmt.Errorf("removing vendor directory: %w", err)
	}
	return nil
}

func cleanPacmanCache() error {
	cacheDir := engine.PacmanCacheDir()
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render("No pacman cache to clean"))
		return nil
	}

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", cacheDir)))
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("removing pacman cache: %w", err)
	}
	return nil
}
