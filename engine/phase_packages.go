package engine

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phasePackages(ctx *actions.BuildContext, rootfs string) error {
	if len(ctx.Packages) == 0 {
		return nil
	}

	src, err := PackageSourceFor(ctx.Arch)
	if err != nil {
		return err
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

	// Initialize the pacman keyring on the host before pacstrap.
	// Uses vendored gpg + gpg-agent + the target distro's keyring package
	// so this works on any host without a host pacman installation.
	gpgDir, err := os.MkdirTemp("", "starforge-keyring-*")
	if err != nil {
		return fmt.Errorf("creating keyring dir: %w", err)
	}
	defer os.RemoveAll(gpgDir)

	confFile, err := src.PacmanConf(cacheDir, gpgDir)
	if err != nil {
		return fmt.Errorf("creating pacman.conf: %w", err)
	}
	defer os.Remove(confFile)

	out.Styled(
		fmt.Sprintf("    pacman-key %s", dimStyle.Render("--init, --populate "+src.KeyringName())),
		fmt.Sprintf("    pacman-key --init, --populate %s", src.KeyringName()),
	)
	if err := initKeyring(gpgDir, confFile, src); err != nil {
		return err
	}

	// Install unpinned packages via pacstrap
	if len(unpinned) > 0 {
		names := make([]string, len(unpinned))
		for i, pkg := range unpinned {
			names[i] = pkg.Name
		}
		out.Styled(
			fmt.Sprintf("    pacstrap %s", dimStyle.Render(strings.Join(names, ", "))),
			fmt.Sprintf("    pacstrap %s", strings.Join(names, ", ")),
		)

		// -C: use our generated config (CacheDir, GPGDir, repos)
		// -c: use host cache mode (absolute host path for --cachedir)
		// -G: skip copying host pacman keyring (we initialized our own above)
		// -M: skip copying host mirrorlist (our config uses direct Server entries)
		args := append([]string{"-C", confFile, "-c", "-G", "-M", rootfs}, names...)
		if err := run("pacstrap", args...); err != nil {
			return err
		}
	} else {
		// Even with no unpinned packages, we need a base rootfs for arch-chroot.
		// pacstrap with no packages just sets up the base filesystem.
		out.Styled(
			fmt.Sprintf("    pacstrap %s", dimStyle.Render("(base only)")),
			"    pacstrap (base only)",
		)
		if err := run("pacstrap", "-C", confFile, "-c", "-G", "-M", rootfs); err != nil {
			return err
		}
	}

	// pacstrap -M skips copying the host mirrorlist and the stock
	// pacman-mirrorlist package ships with all servers commented out.
	// Write a working mirrorlist so the installed system can use pacman.
	mirrorlist := filepath.Join(rootfs, "etc", "pacman.d", "mirrorlist")
	mirrorContent := fmt.Sprintf("Server = %s\n", src.ServerTemplate())
	if err := os.WriteFile(mirrorlist, []byte(mirrorContent), 0o644); err != nil {
		return fmt.Errorf("writing mirrorlist: %w", err)
	}

	// Re-initialize a proper system keyring inside the chroot so the
	// installed OS has its own master key and locally-signed trust chain.
	// The chroot has gnupg + the target distro's keyring package from
	// pacstrap.
	if err := ChrootRun(rootfs, "pacman-key", "--init"); err != nil {
		return fmt.Errorf("system pacman-key --init: %w", err)
	}
	if err := ChrootRun(rootfs, "pacman-key", "--populate", src.KeyringName()); err != nil {
		return fmt.Errorf("system pacman-key --populate: %w", err)
	}

	// pacman-key --populate forks gpg-agent as a background daemon inside
	// the chroot. If left running, it holds the overlay's bind-mounts open
	// (proc, dev, etc.), preventing a clean umount. The lazy fallback umount
	// then leaves the phase-1 upperdir marked "in use" by the kernel's
	// overlayfs, causing EBUSY when phase 2 tries to use it as a lowerdir.
	// Kill gpg-agent before returning so CommitPhase can unmount cleanly.
	out.Styled(
		fmt.Sprintf("    gpgconf %s", dimStyle.Render("--kill gpg-agent")),
		"    gpgconf --kill gpg-agent",
	)
	run("arch-chroot", rootfs, "gpgconf", "--homedir", "/etc/pacman.d/gnupg", "--kill", "gpg-agent")

	// Install pinned packages from the distro's package archive
	for _, pkg := range pinned {
		out.Styled(
			fmt.Sprintf("    archive %s", dimStyle.Render(pkg.String())),
			fmt.Sprintf("    archive %s", pkg.String()),
		)
		if err := installFromArchive(rootfs, pkg, src); err != nil {
			return err
		}
	}

	return nil
}

// installFromArchive installs a specific package version from the distro's
// package archive. If the version doesn't include a pkgrel (no "-"), the
// latest pkgrel is resolved automatically from the archive listing.
// Tries the source's archive arches in order, typically x86_64 then "any".
func installFromArchive(rootfs string, pkg actions.Package, src PackageSource) error {
	archiveURL := src.ArchiveBaseURL()
	if archiveURL == "" {
		return fmt.Errorf("pinned package %s=%s is not supported on %s (no package archive); use an unpinned package",
			pkg.Name, pkg.Version, src.Arch())
	}
	version := pkg.Version

	// Auto-resolve pkgrel if not explicitly provided
	if !strings.Contains(version, "-") {
		resolved, err := resolveLatestPkgrel(pkg.Name, version, src)
		if err != nil {
			return err
		}
		out.SubInfo("resolved %s=%s → %s=%s", pkg.Name, version, pkg.Name, resolved)
		version = resolved
	}

	arches := src.ArchiveArches()
	for _, arch := range arches {
		url := fmt.Sprintf("%s/%s/%s/%s-%s-%s.pkg.tar.zst",
			archiveURL, string(pkg.Name[0]), pkg.Name, pkg.Name, version, arch)
		if err := ChrootRun(rootfs, "pacman", "-U", url, "--noconfirm"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("package %s=%s not found in archive (tried %s)", pkg.Name, version, strings.Join(arches, " and "))
}

// resolveLatestPkgrel fetches the archive directory listing for a package
// and finds the highest pkgrel for the given version.
// e.g. version "5.85" with entries 5.85-1, 5.85-2 → returns "5.85-2".
func resolveLatestPkgrel(name, version string, src PackageSource) (string, error) {
	dirURL := fmt.Sprintf("%s/%s/%s/", src.ArchiveBaseURL(), string(name[0]), name)
	archAlternation := strings.Join(src.ArchiveArches(), "|")
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
		regexp.QuoteMeta(name+"-"+version) + `\-(\d+)\-(?:` + archAlternation + `)\.pkg\.tar\.zst"`,
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

// initKeyring initializes a pacman keyring at gpgDir using vendored tools.
// Writes a gpg.conf that directs GnuPG to our vendored gpg-agent (the
// compiled-in path won't exist on non-Arch hosts), then runs pacman-key
// --init (generates master signing key) and --populate (imports + lsigns
// the distro's developer keys from the vendored keyring package).
func initKeyring(gpgDir, confFile string, src PackageSource) error {
	// Override GnuPG's compiled-in gpg-agent path so it finds our vendored one.
	gpgConf := filepath.Join(gpgDir, "gpg.conf")
	agentPath := filepath.Join(VendorBinDir(), "gpg-agent")
	if err := os.WriteFile(gpgConf, []byte("agent-program "+agentPath+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing gpg.conf: %w", err)
	}

	pacmanKey := resolveBin("pacman-key")
	keyringsDir := filepath.Join(VendorDir(), "usr", "share", "pacman", "keyrings")

	// pacman-key --init: create the master signing key (needs gpg-agent)
	cmd := exec.Command(pacmanKey, "--gpgdir", gpgDir, "--config", confFile, "--init")
	cmd.Env = vendorEnv()
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pacman-key --init: %w", err)
	}

	// pacman-key --populate: import keys from the vendored keyring package
	// and locally sign the trusted ones. --populate-from overrides the
	// default /usr/share/pacman/keyrings/ which won't exist on non-Arch.
	cmd = exec.Command(pacmanKey, "--gpgdir", gpgDir, "--config", confFile,
		"--populate-from", keyringsDir, "--populate", src.KeyringName())
	cmd.Env = vendorEnv()
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pacman-key --populate: %w", err)
	}

	return nil
}
