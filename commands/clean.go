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
	starforge clean <target> cache      Remove only the incremental build cache
	starforge clean <target> artifacts  Remove only sealed disk artifacts
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
	case "artifacts":
		return cleanArtifacts(buildDir, targetName)
	default:
		return fmt.Errorf("unknown scope %q — must be 'cache' or 'artifacts'", scope)
	}
}

func cleanTarget(buildDir, targetName string) error {
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("Nothing to clean for target %q", targetName)))
		return nil
	}

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", buildDir)))
	if err := os.RemoveAll(buildDir); err != nil {
		return cleanPermissionError("build directory", err)
	}
	return nil
}

func cleanCache(buildDir, targetName string) error {
	cacheDir := filepath.Join(buildDir, "cache")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("No cache to clean for target %q", targetName)))
		return nil
	}

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", cacheDir)))
	if err := os.RemoveAll(cacheDir); err != nil {
		return cleanPermissionError("cache", err)
	}
	return nil
}

func cleanArtifacts(buildDir, targetName string) error {
	artifactDir := filepath.Join(buildDir, "artifacts")
	if _, err := os.Stat(artifactDir); os.IsNotExist(err) {
		fmt.Println(cmdDim.Render(fmt.Sprintf("No sealed artifacts to clean for target %q", targetName)))
		return nil
	}

	fmt.Println(cmdSuccess.Render(fmt.Sprintf("Removing %s", artifactDir)))
	if err := os.RemoveAll(artifactDir); err != nil {
		return cleanPermissionError("artifacts", err)
	}
	return nil
}

func cleanPermissionError(subject string, err error) error {
	if os.IsPermission(err) {
		return fmt.Errorf("removing %s: %w (legacy native-executor files may require a one-time sudo cleanup)", subject, err)
	}
	return fmt.Errorf("removing %s: %w", subject, err)
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
