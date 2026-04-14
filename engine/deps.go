package engine

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// vendorPkg describes an Arch Linux package to vendor.
type vendorPkg struct {
	name   string
	repo   string   // "core" or "extra"
	arch   string   // "x86_64" or "any"
	groups []string // e.g. []string{"build"}, []string{"run"}, []string{"build", "run"}
}

// Arch packages to vendor. These are extracted into ~/.local/share/starforge/
// providing usr/bin/ and usr/lib/ trees.
var vendorPackages = []vendorPkg{
	// Orchestration scripts (pacstrap, arch-chroot, genfstab)
	{"arch-install-scripts", "extra", "any", []string{"build"}},
	// Shell: bash is required by the orchestration scripts and host-side
	// layer-run/layer-script steps. Vendor it so we never rely on the
	// host system's /bin/bash.
	{"bash", "core", "x86_64", []string{"build"}},
	{"ncurses", "core", "x86_64", []string{"build"}}, // bash runtime dep
	// Package manager
	{"pacman", "core", "x86_64", []string{"build"}},
	{"pacman-mirrorlist", "core", "any", []string{"build"}},
	// Pacman deps
	{"gpgme", "core", "x86_64", []string{"build"}},
	{"libassuan", "core", "x86_64", []string{"build"}},
	{"libgpg-error", "core", "x86_64", []string{"build"}},
	{"libarchive", "core", "x86_64", []string{"build"}},
	{"curl", "core", "x86_64", []string{"build"}},
	{"libseccomp", "core", "x86_64", []string{"build"}},
	{"libnghttp2", "core", "x86_64", []string{"build"}},
	{"libnghttp3", "core", "x86_64", []string{"build"}},
	{"libidn2", "core", "x86_64", []string{"build"}},
	{"libpsl", "core", "x86_64", []string{"build"}},
	{"libssh2", "core", "x86_64", []string{"build"}},
	{"brotli", "core", "x86_64", []string{"build"}},
	{"openssl", "core", "x86_64", []string{"build"}},
	// GnuPG (for pacman-key)
	{"gnupg", "core", "x86_64", []string{"build"}},
	{"libgcrypt", "core", "x86_64", []string{"build"}},
	{"libksba", "core", "x86_64", []string{"build"}},
	{"npth", "core", "x86_64", []string{"build"}},
	{"pinentry", "core", "x86_64", []string{"build"}},
	{"gnutls", "core", "x86_64", []string{"build"}},
	{"nettle", "core", "x86_64", []string{"build"}},
	{"sqlite", "core", "x86_64", []string{"build"}},
	{"readline", "core", "x86_64", []string{"build"}},
	// Arch Linux keyring (for pacman-key --populate on non-Arch hosts)
	{"archlinux-keyring", "core", "any", []string{"build"}},
	// Filesystem tools
	{"e2fsprogs", "core", "x86_64", []string{"build"}},
	{"dosfstools", "core", "x86_64", []string{"build"}},
	// Partitioning
	{"gptfdisk", "extra", "x86_64", []string{"build"}},
	{"parted", "extra", "x86_64", []string{"build", "run"}},
	// Core system utilities: mount, umount, losetup, sfdisk, blockdev,
	// findmnt, mkswap, lsblk. util-linux-libs (already below) provides
	// the shared libraries; util-linux adds the binaries.
	{"util-linux", "core", "x86_64", []string{"build"}},
	{"libcap", "core", "x86_64", []string{"build"}}, // util-linux dep
	{"pcre2", "core", "x86_64", []string{"build"}},  // util-linux dep
	// Shared library deps for above tools
	{"util-linux-libs", "core", "x86_64", []string{"build"}},
	{"popt", "core", "x86_64", []string{"build"}},
	{"device-mapper", "core", "x86_64", []string{"build", "run"}},
	// UEFI firmware for QEMU
	{"edk2-ovmf", "extra", "any", []string{"run"}},
}

// vendorCheck describes a file whose presence indicates that a group's
// packages have been extracted.
type vendorCheck struct {
	path   string   // relative to VendorDir(), e.g. "usr/bin/pacstrap"
	groups []string
}

// vendorChecks are the files we expect after vendoring each group.
var vendorChecks = []vendorCheck{
	// Orchestration
	{"usr/bin/pacstrap", []string{"build"}},
	{"usr/bin/arch-chroot", []string{"build"}},
	{"usr/bin/genfstab", []string{"build"}},
	// Shell (must come from vendor, never from host)
	{"usr/bin/bash", []string{"build"}},
	// Package manager
	{"usr/bin/pacman", []string{"build"}},
	// Filesystem formatting
	{"usr/bin/mkfs.ext4", []string{"build"}},
	{"usr/bin/mkfs.vfat", []string{"build"}},
	{"usr/bin/e2fsck", []string{"build"}},
	// Partitioning and block device tools (from util-linux)
	{"usr/bin/sfdisk", []string{"build"}},
	{"usr/bin/mount", []string{"build"}},
	{"usr/bin/losetup", []string{"build"}},
	{"usr/bin/blockdev", []string{"build"}},
	{"usr/bin/findmnt", []string{"build"}},
	// Shared device-mapper + parted tools
	{"usr/bin/partprobe", []string{"build", "run"}},
	{"usr/bin/dmsetup", []string{"run"}},
	// Keyring + firmware
	{"usr/share/pacman/keyrings/archlinux.gpg", []string{"build"}},
	{"usr/share/edk2/x64/OVMF_CODE.4m.fd", []string{"run"}},
}

const archMirror = "https://geo.mirror.pkgbuild.com"

// VendorDir returns the path to the vendored dependencies directory.
func VendorDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "starforge")
}

// PacmanCacheDir returns the path to the persistent pacman package cache.
// Uses XDG_STATE_HOME (~/.local/state) to survive clean builds.
func PacmanCacheDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "starforge", "pacman")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "starforge", "pacman")
}

// VendorBinDir returns the path to vendored binaries.
func VendorBinDir() string {
	return filepath.Join(VendorDir(), "usr", "bin")
}

// VendorLibDir returns the path to vendored libraries.
func VendorLibDir() string {
	return filepath.Join(VendorDir(), "usr", "lib")
}

// EnsureDeps checks if vendored dependencies for the requested groups are
// present and downloads them if not. Groups are "build" and "run".
func EnsureDeps(groups ...string) error {
	vendorDir := VendorDir()

	if allGroupChecksPresent(vendorDir, groups) {
		return nil
	}

	out.Header("Installing dependencies")
	out.Styled(
		fmt.Sprintf("  target: %s", vendorDir),
		fmt.Sprintf("  target: %s", vendorDir),
	)

	cacheDir := filepath.Join(vendorDir, "pkg")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating vendor directory: %w", err)
	}

	// Download and extract only packages matching the requested groups
	for _, pkg := range vendorPackages {
		if !matchesAnyGroup(pkg.groups, groups) {
			continue
		}

		if err := out.RunWithSpinner(pkg.name, func() error {
			pkgURL, err := resolvePackageURL(pkg)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", pkg.name, err)
			}

			cachePath := filepath.Join(cacheDir, filepath.Base(pkgURL))
			if _, err := os.Stat(cachePath); err != nil {
				if err := downloadFile(pkgURL, cachePath); err != nil {
					return fmt.Errorf("downloading %s: %w", pkg.name, err)
				}
			}

			return extractPkgTarZst(cachePath, vendorDir)
		}); err != nil {
			return err
		}
	}

	// Patch pacstrap only if build group was requested
	if containsGroup(groups, "build") {
		if err := patchPacstrap(VendorBinDir()); err != nil {
			return fmt.Errorf("patching scripts: %w", err)
		}
	}

	// Verify only checks relevant to the requested groups
	missing := checkGroupMissing(vendorDir, groups)
	if len(missing) > 0 {
		return fmt.Errorf("vendoring incomplete, missing: %s", strings.Join(missing, ", "))
	}

	out.Blank()
	return nil
}

// matchesAnyGroup returns true if the package's groups overlap with the
// requested groups.
func matchesAnyGroup(pkgGroups, requested []string) bool {
	return slices.ContainsFunc(pkgGroups, func(pg string) bool {
		return slices.Contains(requested, pg)
	})
}

// containsGroup returns true if the group list contains the given group.
func containsGroup(groups []string, group string) bool {
	return slices.Contains(groups, group)
}

// allGroupChecksPresent returns true if all vendorChecks matching the
// requested groups are present on disk.
func allGroupChecksPresent(vendorDir string, groups []string) bool {
	for _, vc := range vendorChecks {
		if !matchesAnyGroup(vc.groups, groups) {
			continue
		}
		if _, err := os.Stat(filepath.Join(vendorDir, vc.path)); err != nil {
			return false
		}
	}
	return true
}

// checkGroupMissing returns a list of missing files for the requested groups.
func checkGroupMissing(vendorDir string, groups []string) []string {
	var missing []string
	for _, vc := range vendorChecks {
		if !matchesAnyGroup(vc.groups, groups) {
			continue
		}
		if _, err := os.Stat(filepath.Join(vendorDir, vc.path)); err != nil {
			missing = append(missing, vc.path)
		}
	}
	return missing
}

// patchPacstrap modifies vendored shell scripts to use the vendored bash and
// vendored PATH. Two things are patched:
//  1. The shebang is rewritten from #!/bin/bash to the absolute path of the
//     vendored bash, so the kernel exec uses our bash even when executing the
//     script directly (shebang bypass PATH).
//  2. A PATH export is injected after the shebang so child processes spawned
//     by the script also find vendored binaries first.
func patchPacstrap(binDir string) error {
	vendoredBash := filepath.Join(binDir, "bash")
	for _, script := range []string{"pacstrap", "arch-chroot", "genfstab", "pacman-key"} {
		path := filepath.Join(binDir, script)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		content := string(data)
		marker := "# starforge-patched"
		if strings.Contains(content, marker) {
			continue
		}

		// Split at first newline to isolate the shebang line.
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) != 2 {
			continue
		}

		// Rewrite shebang to use vendored bash so exec() uses our binary.
		shebang := lines[0]
		if shebang == "#!/bin/bash" || shebang == "#!/usr/bin/bash" || shebang == "#!/usr/bin/env bash" {
			shebang = "#!" + vendoredBash
		}

		patched := fmt.Sprintf("%s\n%s\nexport PATH=\"%s:$PATH\"\n%s",
			shebang, marker, binDir, lines[1])
		if err := os.WriteFile(path, []byte(patched), 0o755); err != nil {
			return fmt.Errorf("writing %s: %w", script, err)
		}
	}
	return nil
}

// archPkgInfo is the JSON response from the Arch Linux package API.
type archPkgInfo struct {
	Filename string `json:"filename"`
}

// resolvePackageURL queries the Arch Linux API to get the download URL for a package.
func resolvePackageURL(pkg vendorPkg) (string, error) {
	apiURL := fmt.Sprintf("https://archlinux.org/packages/%s/%s/%s/json/", pkg.repo, pkg.arch, pkg.name)

	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned %d for %s", resp.StatusCode, pkg.name)
	}

	var info archPkgInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("parsing API response: %w", err)
	}

	return fmt.Sprintf("%s/%s/os/x86_64/%s", archMirror, pkg.repo, info.Filename), nil
}

// downloadFile downloads a URL to a local file.
// If the download fails, any partially-written file is removed so that
// a subsequent call does not mistake it for a valid cached download.
func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()

	if copyErr != nil {
		os.Remove(dest) // delete partial file so it is not cached
		return copyErr
	}
	return closeErr
}

// extractPkgTarZst extracts an Arch Linux .pkg.tar.zst package into destDir.
func extractPkgTarZst(pkgPath, destDir string) error {
	f, err := os.Open(pkgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zr, err := zstd.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating zstd reader: %w", err)
	}
	defer zr.Close()

	tr := tar.NewReader(zr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Skip pacman metadata files
		if strings.HasPrefix(header.Name, ".") {
			continue
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			// Validate the symlink target: absolute targets are fine inside
			// the vendor tree, but relative targets must not escape destDir.
			linkTarget := header.Linkname
			if !filepath.IsAbs(linkTarget) {
				resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkTarget))
				rel, err := filepath.Rel(destDir, resolved)
				if err != nil || strings.HasPrefix(rel, "..") {
					return fmt.Errorf("tar: symlink %q target %q escapes vendor directory", header.Name, linkTarget)
				}
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			os.Remove(target)
			if err := os.Symlink(linkTarget, target); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			linkTarget := filepath.Join(destDir, header.Linkname)
			os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				return err
			}
		}
	}

	return nil
}
