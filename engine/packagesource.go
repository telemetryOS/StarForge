package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	archMirrorURL  = "https://geo.mirror.pkgbuild.com"
	archArchiveURL = "https://archive.archlinux.org/packages"
	alarmMirrorURL = "http://mirror.archlinuxarm.org"
	archAPIURLBase = "https://archlinux.org"
)

// PackageSource abstracts the target distro's package configuration for the
// packages phase. Arch Linux and Arch Linux ARM are effectively separate
// distros from a tooling perspective: different mirrors, a different repo
// URL layout, different keyrings, a different package API, and no shared
// versioned archive. Implementations:
//
//   - archLinuxSource: x86_64 Arch Linux (geo.mirror.pkgbuild.com)
//   - alarmSource:     aarch64 Arch Linux ARM (mirror.archlinuxarm.org)
type PackageSource interface {
	// Arch is the target machine architecture ("x86_64" or "aarch64").
	Arch() string

	// PacmanArchitecture is the `Architecture` value written to the
	// generated pacman.conf. Arch Linux keeps "auto" (host-resolved);
	// Arch Linux ARM pins the target arch so an x86_64 build host fetches
	// aarch64 packages.
	PacmanArchitecture() string

	// MirrorURL is the base mirror URL.
	MirrorURL() string

	// ServerTemplate is the pacman Server line body with $repo and $arch
	// placeholders left for pacman to substitute. The two distros order
	// these differently: Arch uses "{mirror}/$repo/os/$arch", Arch Linux
	// ARM uses "{mirror}/$arch/$repo".
	ServerTemplate() string

	// Repos lists the repositories in preference order.
	Repos() []string

	// KeyringPackage is the package providing the distro signing keys
	// (vendored on the build host for pacman-key --populate).
	KeyringPackage() string

	// KeyringName is the argument to `pacman-key --populate`; the vendored
	// keyring file is usr/share/pacman/keyrings/<KeyringName>.gpg.
	KeyringName() string

	// ArchiveBaseURL is the base URL of the distro's versioned package
	// archive used for pinned installs, or empty if the distro has none.
	ArchiveBaseURL() string

	// ArchiveArches lists the package architectures tried in order when
	// resolving pinned packages from the archive.
	ArchiveArches() []string

	// PackageFileURL resolves the download URL for a package file from the
	// distro's package API or mirror listing.
	PackageFileURL(repo, arch, name string) (string, error)

	// KeyringPackageSHA256 is the pinned sha256 digest of the vendored
	// keyring package archive, verified after download. Empty means no pin
	// (x86_64 packages are fetched over TLS and unpacked as today).
	KeyringPackageSHA256() string

	// PacmanConf writes a temporary pacman.conf with the given cache and
	// GPG directories, generated from this source's repositories.
	PacmanConf(cacheDir, gpgDir string) (string, error)
}

// PackageSourceFor returns the package source for the given target arch.
// An empty arch means the default (x86_64 Arch Linux).
func PackageSourceFor(arch string) (PackageSource, error) {
	switch arch {
	case "", "x86_64":
		return archLinuxSource{}, nil
	case "aarch64":
		return alarmSource{}, nil
	default:
		return nil, fmt.Errorf("unsupported target arch %q (supported: x86_64, aarch64)", arch)
	}
}

// goArchByPacmanArch maps pacman architecture strings to Go GOARCH values.
var goArchByPacmanArch = map[string]string{
	"x86_64":  "amd64",
	"aarch64": "arm64",
}

// RequireBinfmt fails loudly when the target arch differs from the build
// host and no binfmt handler is registered. Build phases chroot into the
// target rootfs (pacman-key, locale-gen, useradd, systemctl, bootctl), so
// without qemu-user emulation the target's binaries fail to exec.
func RequireBinfmt(arch string) error {
	goarch := goArchByPacmanArch[arch]
	if goarch == "" {
		return fmt.Errorf("unknown target arch %q", arch)
	}
	if runtime.GOARCH == goarch {
		return nil
	}
	if binfmtHandlerRegistered(arch) {
		return nil
	}
	return fmt.Errorf("target arch %s cannot chroot on this %s host: binfmt emulation is not registered — install qemu-user with binfmt support (e.g. qemu-user-static) or build on a %s host", arch, runtime.GOARCH, arch)
}

// binfmtHandlerRegistered reports whether a binfmt_misc handler exists for
// the given pacman architecture (e.g. qemu-aarch64, qemu-x86_64).
func binfmtHandlerRegistered(arch string) bool {
	entries, err := os.ReadDir("/proc/sys/fs/binfmt_misc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "qemu-"+arch) {
			return true
		}
	}
	return false
}

// --- x86_64 Arch Linux ---

// archLinuxSource serves x86_64 Arch Linux package configuration. URL fields
// are injectable for tests; zero values select the production defaults.
type archLinuxSource struct {
	mirrorURL   string
	archiveBase string
	apiBase     string
}

func (s archLinuxSource) Arch() string               { return "x86_64" }
func (s archLinuxSource) PacmanArchitecture() string { return "auto" }
func (s archLinuxSource) MirrorURL() string {
	if s.mirrorURL != "" {
		return s.mirrorURL
	}
	return archMirrorURL
}
func (s archLinuxSource) ServerTemplate() string {
	return s.MirrorURL() + "/$repo/os/$arch"
}
func (s archLinuxSource) Repos() []string        { return []string{"core", "extra"} }
func (s archLinuxSource) KeyringPackage() string { return "archlinux-keyring" }
func (s archLinuxSource) KeyringName() string    { return "archlinux" }
func (s archLinuxSource) ArchiveBaseURL() string {
	if s.archiveBase != "" {
		return s.archiveBase
	}
	return archArchiveURL
}
func (s archLinuxSource) ArchiveArches() []string      { return []string{"x86_64", "any"} }
func (s archLinuxSource) KeyringPackageSHA256() string { return "" } // TLS transport, floating latest
func (s archLinuxSource) PacmanConf(cacheDir, gpgDir string) (string, error) {
	return writePacmanConf(s, cacheDir, gpgDir)
}

// archPkgInfo is the JSON response from the Arch Linux package API.
type archPkgInfo struct {
	Filename string `json:"filename"`
}

// PackageFileURL queries the archlinux.org JSON API for the current package
// filename and serves it from the mirror's x86_64 tree (arch "any" packages
// also live there).
func (s archLinuxSource) PackageFileURL(repo, arch, name string) (string, error) {
	apiBase := s.apiBase
	if apiBase == "" {
		apiBase = archAPIURLBase
	}
	resp, err := http.Get(fmt.Sprintf("%s/packages/%s/%s/%s/json/", apiBase, repo, arch, name))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned %d for %s", resp.StatusCode, name)
	}

	var info archPkgInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("parsing API response: %w", err)
	}

	return fmt.Sprintf("%s/%s/os/x86_64/%s", s.MirrorURL(), repo, info.Filename), nil
}

// --- aarch64 Arch Linux ARM ---

// alarmSource serves aarch64 Arch Linux ARM package configuration. The mirror
// is injectable for tests; the zero value selects the production geo mirror.
type alarmSource struct {
	mirrorURL string
}

func (s alarmSource) Arch() string               { return "aarch64" }
func (s alarmSource) PacmanArchitecture() string { return "aarch64" }
func (s alarmSource) MirrorURL() string {
	if s.mirrorURL != "" {
		return s.mirrorURL
	}
	return alarmMirrorURL
}
func (s alarmSource) ServerTemplate() string  { return s.MirrorURL() + "/$arch/$repo" }
func (s alarmSource) Repos() []string         { return []string{"core", "extra"} }
func (s alarmSource) KeyringPackage() string  { return "archlinuxarm-keyring" }
func (s alarmSource) KeyringName() string     { return "archlinuxarm" }
func (s alarmSource) ArchiveBaseURL() string  { return "" } // no versioned archive
func (s alarmSource) ArchiveArches() []string { return nil }

// KeyringPackageSHA256 pins the vendored archlinuxarm-keyring package: the
// ALARM geo mirror is plain HTTP (its HTTPS host has a certificate mismatch),
// so the trust anchor must be digest-pinned. If the distro publishes a newer
// keyring, bump the expected filename resolution and this digest together.
func (s alarmSource) KeyringPackageSHA256() string {
	return "3cb36869edfe413672a6e932cc55d7f8386e1a9d3b38663cfb3bc6fe0d146e21" // archlinuxarm-keyring-20240419-2-any.pkg.tar.xz
}

func (s alarmSource) PacmanConf(cacheDir, gpgDir string) (string, error) {
	return writePacmanConf(s, cacheDir, gpgDir)
}

// PackageFileURL resolves a package file from the ALARM mirror's repo
// directory listing. ALARM has no JSON package API; the listing HTML
// contains entries like `archlinuxarm-keyring-20240419-2-any.pkg.tar.xz`.
// The geo mirror redirects to a regional mirror (plain HTTP — its HTTPS
// host has a certificate mismatch), which net/http follows automatically.
func (s alarmSource) PackageFileURL(repo, pkgArch, name string) (string, error) {
	// The package file lives in the target arch's repo directory
	// (aarch64/core/), while the filename carries the package arch
	// ("any" for the keyring package).
	targetArch := s.Arch()
	listingURL := fmt.Sprintf("%s/%s/%s/", s.MirrorURL(), targetArch, repo)

	resp, err := http.Get(listingURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("mirror listing returned %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading mirror listing: %w", err)
	}

	// Match `name-version-pkgrel-arch.pkg.tar.{xz,zst}` hrefs.
	pattern := regexp.MustCompile(
		regexp.QuoteMeta(name) + `\-([0-9][0-9A-Za-z_.]*)\-(\d+)\-` + regexp.QuoteMeta(pkgArch) + `\.pkg\.tar\.(xz|zst)"`,
	)

	matches := pattern.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no package file found for %s in %s listing", name, listingURL)
	}

	best := matches[0]
	for _, m := range matches[1:] {
		if comparePkgVersion(m[1], best[1]) > 0 ||
			(m[1] == best[1] && atoiDefault(m[2]) > atoiDefault(best[2])) {
			best = m
		}
	}

	return fmt.Sprintf("%s/%s/%s/%s-%s-%s-%s.pkg.tar.%s",
		s.MirrorURL(), targetArch, repo, name, best[1], best[2], pkgArch, best[3]), nil
}

// comparePkgVersion compares two pacman version strings, numerically when
// both are numeric (e.g. ALARM keyring dates like 20240419), otherwise
// lexicographically.
func comparePkgVersion(a, b string) int {
	if an, aerr := strconv.Atoi(a); aerr == nil {
		if bn, berr := strconv.Atoi(b); berr == nil {
			switch {
			case an < bn:
				return -1
			case an > bn:
				return 1
			default:
				return 0
			}
		}
	}
	return strings.Compare(a, b)
}

func atoiDefault(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// writePacmanConf renders the shared pacman.conf template for a source and
// writes it to a temporary file.
func writePacmanConf(src PackageSource, cacheDir, gpgDir string) (string, error) {
	conf := fmt.Sprintf(`[options]
HoldPkg = pacman glibc
Architecture = %s
SigLevel = Required DatabaseOptional
CacheDir = %s
GPGDir = %s
`, src.PacmanArchitecture(), cacheDir, gpgDir)

	for _, repo := range src.Repos() {
		conf += fmt.Sprintf("\n[%s]\nServer = %s\n", repo, src.ServerTemplate())
	}

	f, err := os.CreateTemp("", "starforge-pacman-*.conf")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(conf); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}
