package engine

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// vendorPkg describes an Arch Linux package to vendor.
type vendorPkg struct {
	name   string
	repo   string        // "core" or "extra"
	arch   string        // "x86_64", "aarch64", or "any"
	groups []string      // e.g. []string{"build"}, []string{"run"}, []string{"build", "run"}
	source PackageSource // distro serving the package; nil = x86_64 Arch Linux
	digest string        // pinned sha256 of the package file; empty = unpinned
	url    string        // exact package file URL; empty = resolve dynamically
}

// resolvePkgSource returns the distro a vendored package is fetched from.
// Static entries are host tooling served by x86_64 Arch Linux regardless of
// the target arch; per-target entries (keyrings) set source explicitly.
func (p vendorPkg) resolvePkgSource() PackageSource {
	if p.source != nil {
		return p.source
	}
	return archLinuxSource{}
}

// Arch packages to vendor. These are extracted into ~/.local/share/starforge/
// providing usr/bin/ and usr/lib/ trees.
var vendorPackages = []vendorPkg{
	// Orchestration scripts (pacstrap, arch-chroot)
	{"arch-install-scripts", "extra", "any", []string{"build"}, nil, "", ""},
	// Shell: bash is required by the orchestration scripts and host-side
	// layer-run/layer-script steps. Vendor it so we never rely on the
	// host system's /bin/bash.
	{"bash", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"ncurses", "core", "x86_64", []string{"build"}, nil, "", ""}, // bash runtime dep
	// Package manager
	{"pacman", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"pacman-mirrorlist", "core", "any", []string{"build"}, nil, "", ""},
	// Pacman deps
	{"gpgme", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libassuan", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libgpg-error", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libarchive", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"curl", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libseccomp", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libnghttp2", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libnghttp3", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libidn2", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libpsl", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libssh2", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"brotli", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"openssl", "core", "x86_64", []string{"build"}, nil, "", ""},
	// GnuPG (for pacman-key)
	{"gnupg", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libgcrypt", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libksba", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"npth", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"pinentry", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"gnutls", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"nettle", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"sqlite", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"readline", "core", "x86_64", []string{"build"}, nil, "", ""},
	// Filesystem tools
	{"e2fsprogs", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"dosfstools", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"zstd", "core", "x86_64", []string{"build"}, nil, "", ""},
	// Partitioning
	{"gptfdisk", "extra", "x86_64", []string{"build"}, nil, "", ""},
	{"parted", "extra", "x86_64", []string{"build", "run"}, nil, "", ""},
	// Core system utilities: mount, umount, losetup, sfdisk, blockdev,
	// findmnt, mkswap, lsblk. util-linux-libs (already below) provides
	// the shared libraries; util-linux adds the binaries.
	{"util-linux", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"libcap", "core", "x86_64", []string{"build"}, nil, "", ""}, // util-linux dep
	{"pcre2", "core", "x86_64", []string{"build"}, nil, "", ""},  // util-linux dep
	// Shared library deps for above tools
	{"util-linux-libs", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"popt", "core", "x86_64", []string{"build"}, nil, "", ""},
	{"device-mapper", "core", "x86_64", []string{"build", "run"}, nil, "", ""},
	// UEFI firmware for QEMU
	{"edk2-ovmf", "extra", "any", []string{"run"}, nil, "", ""},
}

// Target keyrings are vendored per build: the target distro's signing keys
// must be present on the build host before pacstrap verifies packages.
func keyringVendorPkg(src PackageSource) vendorPkg {
	return vendorPkg{
		name:   src.KeyringPackage(),
		repo:   "core",
		arch:   "any",
		groups: []string{"build"},
		source: src,
		digest: src.KeyringPackageSHA256(),
		url:    src.KeyringPackageURL(),
	}
}

// keyringVendorCheck is the extracted keyring file the vendored
// pacman-key --populate reads.
func keyringVendorCheck(src PackageSource) vendorCheck {
	return vendorCheck{
		path:   fmt.Sprintf("usr/share/pacman/keyrings/%s.gpg", src.KeyringName()),
		groups: []string{"build"},
	}
}

// verifyPkgDigest checks a downloaded package archive against its pinned
// sha256 digest.
func verifyPkgDigest(pkgPath, want string) error {
	f, err := os.Open(pkgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return err
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != want {
		return fmt.Errorf("digest mismatch (got %s, want %s) — if the distro published a newer keyring, update the pinned KeyringPackageSHA256", got, want)
	}
	return nil
}

// extractKeyringPkg extracts a keyring package isolated from the shared
// vendor tree and copies only usr/share/pacman/keyrings/ files into it.
// The keyring is the trust root for all later signature verification, so a
// tampered archive must not be able to overwrite vendored executables.
func extractKeyringPkg(pkgPath, vendorDir string) error {
	tmp, err := os.MkdirTemp("", "starforge-keyring-pkg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := extractPkg(pkgPath, tmp); err != nil {
		return err
	}

	srcDir := filepath.Join(tmp, "usr", "share", "pacman", "keyrings")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("keyring package missing usr/share/pacman/keyrings/: %w", err)
	}
	destDir := filepath.Join(vendorDir, "usr", "share", "pacman", "keyrings")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			return fmt.Errorf("keyring package contains unexpected entry %q", e.Name())
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destDir, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// vendorCheck describes a file whose presence indicates that a group's
// packages have been extracted.
type vendorCheck struct {
	path   string // relative to VendorDir(), e.g. "usr/bin/pacstrap"
	groups []string
}

// vendorChecks are the files we expect after vendoring each group.
var vendorChecks = []vendorCheck{
	// Orchestration
	{"usr/bin/pacstrap", []string{"build"}},
	{"usr/bin/arch-chroot", []string{"build"}},
	// Shell (must come from vendor, never from host)
	{"usr/bin/bash", []string{"build"}},
	// Package manager
	{"usr/bin/pacman", []string{"build"}},
	// Filesystem formatting
	{"usr/bin/mkfs.ext4", []string{"build"}},
	{"usr/bin/mkfs.vfat", []string{"build"}},
	{"usr/bin/e2fsck", []string{"build"}},
	{"usr/bin/zstd", []string{"build"}},
	// Partitioning and block device tools (from util-linux)
	{"usr/bin/sfdisk", []string{"build"}},
	{"usr/bin/mount", []string{"build"}},
	{"usr/bin/losetup", []string{"build"}},
	{"usr/bin/blockdev", []string{"build"}},
	{"usr/bin/findmnt", []string{"build"}},
	// Shared device-mapper + parted tools
	{"usr/bin/partprobe", []string{"build", "run"}},
	{"usr/bin/dmsetup", []string{"run"}},
	// Firmware
	{"usr/share/edk2/x64/OVMF_CODE.4m.fd", []string{"run"}},
}

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
// present and downloads them if not. Groups are "build" and "run". The
// target arch selects which distro keyring is vendored for the build group
// (host tooling is always x86_64 Arch Linux).
func EnsureDeps(arch string, groups ...string) error {
	src, err := PackageSourceFor(arch)
	if err != nil {
		return err
	}
	vendorDir := VendorDir()

	// The keyring package and its extracted-file check come from the
	// target distro; all other vendor packages and checks are static.
	pkgs := append([]vendorPkg(nil), vendorPackages...)
	checks := append([]vendorCheck(nil), vendorChecks...)
	if containsGroup(groups, "build") {
		pkgs = append(pkgs, keyringVendorPkg(src))
		checks = append(checks, keyringVendorCheck(src))
	}

	if allGroupChecksPresent(vendorDir, checks, groups) {
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
	for _, pkg := range pkgs {
		if !matchesAnyGroup(pkg.groups, groups) {
			continue
		}

		if err := out.RunWithSpinner(pkg.name, func() error {
			pkgURL := pkg.url
			if pkgURL == "" {
				var err error
				pkgURL, err = pkg.resolvePkgSource().PackageFileURL(pkg.repo, pkg.arch, pkg.name)
				if err != nil {
					return fmt.Errorf("resolving %s: %w", pkg.name, err)
				}
			}
			cachePath := filepath.Join(cacheDir, filepath.Base(pkgURL))
			if err := fetchVendorPackage(pkgURL, cachePath, pkg.digest); err != nil {
				return fmt.Errorf("%s: %w", pkg.name, err)
			}

			// The keyring package is the trust root for every later
			// verification, so it must not be able to overwrite vendored
			// executables: extract it isolated and copy only keyring files.
			if pkg.source != nil {
				return extractKeyringPkg(cachePath, vendorDir)
			}
			return extractPkg(cachePath, vendorDir)
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
	missing := checkGroupMissing(vendorDir, checks, groups)
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

// allGroupChecksPresent returns true if all checks matching the requested
// groups are present on disk.
func allGroupChecksPresent(vendorDir string, checks []vendorCheck, groups []string) bool {
	for _, vc := range checks {
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
func checkGroupMissing(vendorDir string, checks []vendorCheck, groups []string) []string {
	var missing []string
	for _, vc := range checks {
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
	for _, script := range []string{"pacstrap", "arch-chroot", "pacman-key"} {
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

// downloadFile downloads a URL to a local file.
// If the download fails, any partially-written file is removed so that
// a subsequent call does not mistake it for a valid cached download.
func downloadFile(url, dest string) error {
	// Stage into a sibling temp file and rename only after a complete
	// download, so a partial or oversized response never lands in the cache.
	tmp := dest + ".part"
	os.Remove(tmp)

	resp, err := httpClient().Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(f, io.LimitReader(resp.Body, int64(maxPackageDownloadBytes)+1))
	if copyErr == nil {
		if info, statErr := f.Stat(); statErr == nil && info.Size() > int64(maxPackageDownloadBytes) {
			copyErr = fmt.Errorf("package archive exceeds %d bytes", maxPackageDownloadBytes)
		}
	}
	closeErr := f.Close()

	if copyErr != nil {
		os.Remove(tmp) // delete partial file so it is not cached
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// maxPackageDownloadBytes bounds a vendored package download so a hostile or
// broken mirror cannot exhaust disk (ALARM mirrors are plain HTTP). It is a
// var so tests can lower it.
var maxPackageDownloadBytes = 512 << 20

// maxListingBytes bounds package-API and mirror-listing reads.
const maxListingBytes = 16 << 20

// httpClient returns the shared client used for package API, listing, and
// download requests, with a timeout so a stalled connection fails the build
// instead of hanging it.
func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}

// extractPkg extracts a .pkg.tar.zst or .pkg.tar.xz package into destDir.
// Arch Linux packages are zstd-compressed; Arch Linux ARM still ships xz.
func extractPkg(pkgPath, destDir string) error {
	r, closer, err := openPkgDecompressor(pkgPath)
	if err != nil {
		return err
	}
	defer closer()

	tr := tar.NewReader(r)

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

// openPkgDecompressor sniffs the package compression from its magic bytes
// and returns a tar-ready reader plus a cleanup func.
func openPkgDecompressor(pkgPath string) (io.Reader, func(), error) {
	f, err := os.Open(pkgPath)
	if err != nil {
		return nil, nil, err
	}

	// Peek (not Read) so the header stays in the stream: xz.Reader expects
	// to read the header itself.
	br := bufio.NewReader(f)
	magic, err := br.Peek(6)
	if len(magic) < 4 {
		f.Close()
		return nil, nil, fmt.Errorf("reading %s: truncated or empty package (%w)", pkgPath, err)
	}

	switch {
	case len(magic) >= 4 && bytes.Equal(magic[:4], []byte{0x28, 0xB5, 0x2F, 0xFD}): // zstd
		zr, err := zstd.NewReader(br)
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("creating zstd reader: %w", err)
		}
		return zr, func() { zr.Close(); f.Close() }, nil
	case len(magic) >= 6 && bytes.Equal(magic[:6], []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}): // xz
		xr, err := xz.NewReader(br)
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("creating xz reader: %w", err)
		}
		return xr, func() { f.Close() }, nil
	default:
		f.Close()
		return nil, nil, fmt.Errorf("unsupported package compression in %s", pkgPath)
	}
}

// fetchVendorPackage downloads a package into cachePath when it is not
// cached, then verifies a pinned digest when one is given. A cache entry
// that fails verification is evicted, so a poisoned or stale file cannot
// wedge every later build.
func fetchVendorPackage(pkgURL, cachePath, digest string) error {
	if _, err := os.Stat(cachePath); err != nil {
		if err := downloadFile(pkgURL, cachePath); err != nil {
			return fmt.Errorf("downloading: %w", err)
		}
	}

	if digest != "" {
		if err := verifyPkgDigest(cachePath, digest); err != nil {
			os.Remove(cachePath)
			return err
		}
	}
	return nil
}
