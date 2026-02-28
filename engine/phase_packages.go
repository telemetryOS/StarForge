package engine

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

const archiveURL = "https://archive.archlinux.org/packages"

func (b *Builder) phasePackages(ctx *actions.BuildContext, rootfs string) error {
	if len(ctx.Packages) == 0 {
		return nil
	}

	// Split into unpinned (latest from repos) and pinned (specific version from archive)
	var unpinned, pinned []actions.Package
	for _, pkg := range ctx.Packages {
		if pkg.Version != "" {
			pinned = append(pinned, pkg)
		} else {
			unpinned = append(unpinned, pkg)
		}
	}

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

	// Install unpinned packages via pacstrap
	if len(unpinned) > 0 {
		names := make([]string, len(unpinned))
		for i, pkg := range unpinned {
			names[i] = pkg.Name
		}
		fmt.Printf("    pacstrap %s\n", dimStyle.Render(strings.Join(names, ", ")))

		// -C: use our config with custom CacheDir
		// -c: use host cache mode (passes CacheDir as --cachedir to pacman,
		//     which is an absolute host path rather than relative to rootfs)
		// -K: initialize an empty pacman keyring in the target
		args := append([]string{"-C", confFile, "-c", "-K", rootfs}, names...)
		if err := run("pacstrap", args...); err != nil {
			return err
		}
	} else {
		// Even with no unpinned packages, we need a base rootfs for arch-chroot.
		// pacstrap with no packages just sets up the base filesystem.
		fmt.Printf("    pacstrap %s\n", dimStyle.Render("(base only)"))
		if err := run("pacstrap", "-C", confFile, "-c", "-K", rootfs); err != nil {
			return err
		}
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

	// Install pinned packages from the Arch Linux Archive
	for _, pkg := range pinned {
		fmt.Printf("    archive %s\n", dimStyle.Render(pkg.String()))
		if err := installFromArchive(rootfs, pkg); err != nil {
			return err
		}
	}

	return nil
}

// installFromArchive installs a specific package version from the Arch Linux
// Archive. If the version doesn't include a pkgrel (no "-"), the latest
// pkgrel is resolved automatically from the archive listing.
// Tries x86_64 first, then falls back to any architecture.
func installFromArchive(rootfs string, pkg actions.Package) error {
	version := pkg.Version

	// Auto-resolve pkgrel if not explicitly provided
	if !strings.Contains(version, "-") {
		resolved, err := resolveLatestPkgrel(pkg.Name, version)
		if err != nil {
			return err
		}
		fmt.Printf("      resolved %s=%s → %s=%s\n", pkg.Name, version, pkg.Name, resolved)
		version = resolved
	}

	for _, arch := range []string{"x86_64", "any"} {
		url := fmt.Sprintf("%s/%s/%s/%s-%s-%s.pkg.tar.zst",
			archiveURL, string(pkg.Name[0]), pkg.Name, pkg.Name, version, arch)
		if err := ChrootRun(rootfs, "pacman", "-U", url, "--noconfirm"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("package %s=%s not found in archive (tried x86_64 and any)", pkg.Name, version)
}

// resolveLatestPkgrel fetches the archive directory listing for a package
// and finds the highest pkgrel for the given version.
// e.g. version "5.85" with entries 5.85-1, 5.85-2 → returns "5.85-2".
func resolveLatestPkgrel(name, version string) (string, error) {
	dirURL := fmt.Sprintf("%s/%s/%s/", archiveURL, string(name[0]), name)
	resp, err := http.Get(dirURL)
	if err != nil {
		return "", fmt.Errorf("fetching archive listing for %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("archive listing for %s returned HTTP %d", name, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading archive listing for %s: %w", name, err)
	}

	// Match filenames like: name-version-pkgrel-arch.pkg.tar.zst
	// in the HTML directory listing href attributes.
	pattern := regexp.MustCompile(
		regexp.QuoteMeta(name+"-"+version) + `\-(\d+)\-(?:x86_64|any)\.pkg\.tar\.zst"`,
	)

	matches := pattern.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no archive entries found for %s version %s", name, version)
	}

	maxRel := 0
	for _, m := range matches {
		rel, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if rel > maxRel {
			maxRel = rel
		}
	}

	if maxRel == 0 {
		return "", fmt.Errorf("no valid pkgrel found for %s version %s", name, version)
	}

	return fmt.Sprintf("%s-%d", version, maxRel), nil
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
