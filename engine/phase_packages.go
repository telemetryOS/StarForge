package engine

import (
	"fmt"
	"os"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phasePackages(ctx *actions.BuildContext, rootfs string) error {
	if len(ctx.Packages) == 0 {
		return nil
	}
	fmt.Printf("    pacstrap %s\n", dimStyle.Render(strings.Join(ctx.Packages, ", ")))

	// Use a persistent pacman cache so clean builds don't re-download packages.
	cacheDir := PacmanCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating pacman cache dir: %w", err)
	}

	confFile, err := pacmanConf(cacheDir)
	if err != nil {
		return fmt.Errorf("creating pacman.conf: %w", err)
	}
	defer os.Remove(confFile)

	// -C: use our config with custom CacheDir
	// -c: use host cache mode (passes CacheDir as --cachedir to pacman,
	//     which is an absolute host path rather than relative to rootfs)
	// -K: initialize an empty pacman keyring in the target
	args := append([]string{"-C", confFile, "-c", "-K", rootfs}, ctx.Packages...)
	if err := run("pacstrap", args...); err != nil {
		return err
	}

	// Initialize and populate the pacman keyring so the installed system
	// can verify package signatures without manual key imports.
	fmt.Printf("    pacman-key %s\n", dimStyle.Render("--init, --populate archlinux"))
	if err := run("arch-chroot", rootfs, "pacman-key", "--init"); err != nil {
		return fmt.Errorf("pacman-key --init: %w", err)
	}
	if err := run("arch-chroot", rootfs, "pacman-key", "--populate", "archlinux"); err != nil {
		return fmt.Errorf("pacman-key --populate: %w", err)
	}
	return nil
}

// pacmanConf creates a temporary pacman.conf that uses the given cache directory.
// It includes the system config for repos/mirrors and overrides CacheDir.
func pacmanConf(cacheDir string) (string, error) {
	// Read the system pacman.conf as the base
	base, err := os.ReadFile("/etc/pacman.conf")
	if err != nil {
		return "", fmt.Errorf("reading /etc/pacman.conf: %w", err)
	}

	// Build a config that sets CacheDir before including the rest.
	// pacman uses the last CacheDir directive, but a standalone directive
	// before the [options] section doesn't work — we need to replace the
	// existing CacheDir line or inject into [options].
	content := string(base)
	var result strings.Builder
	replaced := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CacheDir") && strings.Contains(trimmed, "=") {
			result.WriteString(fmt.Sprintf("CacheDir = %s\n", cacheDir))
			replaced = true
			continue
		}
		result.WriteString(line)
		result.WriteString("\n")
		// If we hit [options] and haven't replaced yet, inject CacheDir
		if !replaced && trimmed == "[options]" {
			result.WriteString(fmt.Sprintf("CacheDir = %s\n", cacheDir))
			replaced = true
		}
	}

	f, err := os.CreateTemp("", "starforge-pacman-*.conf")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(result.String()); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}
