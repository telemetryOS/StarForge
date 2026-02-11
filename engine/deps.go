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
	// Filesystem tools
	{"e2fsprogs", "core", "x86_64", []string{"build"}},
	{"dosfstools", "core", "x86_64", []string{"build"}},
	// Partitioning
	{"gptfdisk", "extra", "x86_64", []string{"build"}},
	{"parted", "extra", "x86_64", []string{"build", "run"}},
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
	{"usr/bin/pacstrap", []string{"build"}},
	{"usr/bin/arch-chroot", []string{"build"}},
	{"usr/bin/genfstab", []string{"build"}},
	{"usr/bin/pacman", []string{"build"}},
	{"usr/bin/mkfs.ext4", []string{"build"}},
	{"usr/bin/mkfs.vfat", []string{"build"}},
	{"usr/bin/sgdisk", []string{"build"}},
	{"usr/bin/partprobe", []string{"build", "run"}},
	{"usr/bin/dmsetup", []string{"run"}},
	{"usr/share/edk2/x64/OVMF_CODE.4m.fd", []string{"run"}},
}

const archMirror = "https://geo.mirror.pkgbuild.com"

// VendorDir returns the path to the vendored dependencies directory.
func VendorDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "starforge")
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

	fmt.Println(headerStyle.Render("Installing dependencies"))
	fmt.Printf("  target: %s\n", vendorDir)

	cacheDir := filepath.Join(vendorDir, "pkg")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating vendor directory: %w", err)
	}

	// Download and extract only packages matching the requested groups
	for _, pkg := range vendorPackages {
		if !matchesAnyGroup(pkg.groups, groups) {
			continue
		}

		fmt.Printf("  %s ", pkg.name)

		pkgURL, err := resolvePackageURL(pkg)
		if err != nil {
			fmt.Println("✗")
			return fmt.Errorf("resolving %s: %w", pkg.name, err)
		}

		cachePath := filepath.Join(cacheDir, filepath.Base(pkgURL))
		if _, err := os.Stat(cachePath); err != nil {
			if err := downloadFile(pkgURL, cachePath); err != nil {
				fmt.Println("✗")
				return fmt.Errorf("downloading %s: %w", pkg.name, err)
			}
		}

		if err := extractPkgTarZst(cachePath, vendorDir); err != nil {
			fmt.Println("✗")
			return fmt.Errorf("extracting %s: %w", pkg.name, err)
		}

		fmt.Println("✓")
	}

	// Patch pacstrap only if build group was requested
	if containsGroup(groups, "build") {
		patchPacstrap(VendorBinDir())
	}

	// Verify only checks relevant to the requested groups
	missing := checkGroupMissing(vendorDir, groups)
	if len(missing) > 0 {
		return fmt.Errorf("vendoring incomplete, missing: %s", strings.Join(missing, ", "))
	}

	fmt.Println()
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

// patchPacstrap modifies the vendored pacstrap script to use our pacman binary
// by injecting a PATH override at the top of the script.
func patchPacstrap(binDir string) {
	for _, script := range []string{"pacstrap", "arch-chroot", "genfstab"} {
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

		// Insert PATH export after the shebang line
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) == 2 {
			patched := fmt.Sprintf("%s\n%s\nexport PATH=\"%s:$PATH\"\n%s",
				lines[0], marker, binDir, lines[1])
			os.WriteFile(path, []byte(patched), 0o755)
		}
	}
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
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
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
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			os.Remove(target)
			if err := os.Symlink(header.Linkname, target); err != nil {
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
