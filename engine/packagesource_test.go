package engine

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/ulikunitz/xz"
)

// --- PackageSourceFor ---

func TestPackageSourceFor_DefaultIsArchLinux(t *testing.T) {
	for _, arch := range []string{"", "x86_64"} {
		src, err := PackageSourceFor(arch)
		if err != nil {
			t.Fatalf("arch %q: unexpected error: %v", arch, err)
		}
		if _, ok := src.(archLinuxSource); !ok {
			t.Fatalf("arch %q: expected archLinuxSource, got %T", arch, src)
		}
	}
}

func TestPackageSourceFor_Aarch64IsArchARM(t *testing.T) {
	src, err := PackageSourceFor("aarch64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := src.(alarmSource); !ok {
		t.Fatalf("expected alarmSource, got %T", src)
	}
}

func TestPackageSourceFor_UnsupportedArch(t *testing.T) {
	if _, err := PackageSourceFor("armv7h"); err == nil {
		t.Fatal("expected error for unsupported arch")
	}
}

// --- pacman.conf generation ---

func TestPacmanConf_ArchLinuxMatchesLegacyTemplate(t *testing.T) {
	src, err := PackageSourceFor("x86_64")
	if err != nil {
		t.Fatal(err)
	}
	path, err := src.PacmanConf("/tmp/cache", "/tmp/gpg")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	want := `[options]
HoldPkg = pacman glibc
Architecture = auto
SigLevel = Required DatabaseOptional
CacheDir = /tmp/cache
GPGDir = /tmp/gpg

[core]
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch

[extra]
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch
`
	assertConfFile(t, path, want)
}

func TestPacmanConf_ArchARM(t *testing.T) {
	src, err := PackageSourceFor("aarch64")
	if err != nil {
		t.Fatal(err)
	}
	path, err := src.PacmanConf("/tmp/cache", "/tmp/gpg")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	want := `[options]
HoldPkg = pacman glibc
Architecture = aarch64
SigLevel = Required DatabaseOptional
CacheDir = /tmp/cache
GPGDir = /tmp/gpg

[core]
Server = http://mirror.archlinuxarm.org/$arch/$repo

[extra]
Server = http://mirror.archlinuxarm.org/$arch/$repo

[alarm]
Server = http://mirror.archlinuxarm.org/$arch/$repo

[aur]
Server = http://mirror.archlinuxarm.org/$arch/$repo
`
	assertConfFile(t, path, want)
}

func assertConfFile(t *testing.T, path, want string) {
	t.Helper()
	got := readFile(t, path)
	if got != want {
		t.Fatalf("pacman.conf mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// --- phase 1 cache hash ---

func TestHashPhase_PackagesIncludesArch(t *testing.T) {
	mk := func(arch string) *actions.BuildContext {
		ctx := actions.NewBuildContext()
		ctx.Arch = arch
		ctx.Packages = []actions.Package{{Name: "base"}}
		return ctx
	}

	x86, err := HashPhase(1, mk("x86_64"))
	if err != nil {
		t.Fatal(err)
	}
	arm, err := HashPhase(1, mk("aarch64"))
	if err != nil {
		t.Fatal(err)
	}
	if x86 == arm {
		t.Fatal("expected phase 1 hash to differ between arches")
	}
}

// --- keyring vendoring ---

func TestKeyringVendorPkg_PerSource(t *testing.T) {
	archSrc := keyringVendorPkg(archLinuxSource{})
	alarmPkg := keyringVendorPkg(alarmSource{})

	if archSrc.name != "archlinux-keyring" {
		t.Fatalf("arch keyring package: got %q", archSrc.name)
	}
	if alarmPkg.name != "archlinuxarm-keyring" {
		t.Fatalf("alarm keyring package: got %q", alarmPkg.name)
	}
}

func TestKeyringVendorCheck_PerSource(t *testing.T) {
	archCheck := keyringVendorCheck(archLinuxSource{})
	alarmCheck := keyringVendorCheck(alarmSource{})

	if archCheck.path != "usr/share/pacman/keyrings/archlinux.gpg" {
		t.Fatalf("arch keyring check: got %q", archCheck.path)
	}
	if alarmCheck.path != "usr/share/pacman/keyrings/archlinuxarm.gpg" {
		t.Fatalf("alarm keyring check: got %q", alarmCheck.path)
	}
}

// --- pinned packages on distros without an archive ---

func TestInstallFromArchive_NoArchiveDistroFails(t *testing.T) {
	pkg := actions.Package{Name: "bash", Version: "5.2-1"}
	err := installFromArchive("/unused", pkg, alarmSource{})
	if err == nil {
		t.Fatal("expected error for pinned package on a distro without an archive")
	}
	if !strings.Contains(err.Error(), "no package archive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ALARM mirror listing resolution ---

func TestAlarmPackageFileURL_ResolvesLatestVersion(t *testing.T) {
	listing := `<a href="../">..</a>
<a href="archlinuxarm-keyring-20240419-1-any.pkg.tar.xz">archlinuxarm-keyring-20240419-1-any.pkg.tar.xz</a>
<a href="archlinuxarm-keyring-20240419-2-any.pkg.tar.xz">archlinuxarm-keyring-20240419-2-any.pkg.tar.xz</a>
<a href="archlinuxarm-keyring-20230101-9-any.pkg.tar.xz">archlinuxarm-keyring-20230101-9-any.pkg.tar.xz</a>
<a href="pacman-7.0.0-1-aarch64.pkg.tar.xz">pacman-7.0.0-1-aarch64.pkg.tar.xz</a>
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aarch64/core/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(listing))
	}))
	defer srv.Close()

	src := alarmSource{mirrorURL: srv.URL}
	url, err := src.PackageFileURL("core", "any", "archlinuxarm-keyring")
	if err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/aarch64/core/archlinuxarm-keyring-20240419-2-any.pkg.tar.xz"
	if url != want {
		t.Fatalf("resolved URL: got %s, want %s", url, want)
	}
}

func TestAlarmPackageFileURL_NoMatchFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="other-package-1.0-1-any.pkg.tar.xz">other</a>`))
	}))
	defer srv.Close()

	src := alarmSource{mirrorURL: srv.URL}
	if _, err := src.PackageFileURL("core", "any", "archlinuxarm-keyring"); err == nil {
		t.Fatal("expected error when the package is not in the listing")
	}
}

func TestComparePkgVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"20240419", "20230101", 1},
		{"20230101", "20240419", -1},
		{"20240419", "20240419", 0},
		{"7.0.0", "6.1.0", 1},
	}
	for _, c := range cases {
		if got := comparePkgVersion(c.a, c.b); got != c.want {
			t.Errorf("comparePkgVersion(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// --- package extraction (xz) ---

func TestExtractPkg_XzPackage(t *testing.T) {
	// Build a minimal .pkg.tar.xz with a regular file and a safe symlink.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	body := "keyring-content"
	mustTarEntry(t, tw, &tar.Header{Name: "usr/share/pacman/keyrings/alarm.gpg", Mode: 0o644, Size: int64(len(body))}, body)
	mustTarEntry(t, tw, &tar.Header{Name: "usr/share/link", Typeflag: tar.TypeSymlink, Linkname: "keyrings/alarm.gpg"}, "")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	pkgPath := filepath.Join(t.TempDir(), "test-1.0-1-any.pkg.tar.xz")
	pf, err := os.Create(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	xw, err := xz.NewWriter(pf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := xw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pf.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractPkg(pkgPath, dest); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dest, "usr/share/pacman/keyrings/alarm.gpg"))
	if got != body {
		t.Fatalf("extracted content: got %q", got)
	}
	link, err := os.Readlink(filepath.Join(dest, "usr/share/link"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "keyrings/alarm.gpg" {
		t.Fatalf("extracted symlink: got %q", link)
	}
}

func mustTarEntry(t *testing.T, tw *tar.Writer, h *tar.Header, body string) {
	t.Helper()
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
}

// --- digest pinning + source routing ---

func TestKeyringVendorPkg_DigestAndRouting(t *testing.T) {
	alarmPkg := keyringVendorPkg(alarmSource{})
	if alarmPkg.digest == "" {
		t.Fatal("ALARM keyring must be digest-pinned (plain HTTP transport)")
	}
	if _, ok := alarmPkg.source.(alarmSource); !ok {
		t.Fatalf("ALARM keyring must route through alarmSource, got %T", alarmPkg.source)
	}

	archPkg := keyringVendorPkg(archLinuxSource{})
	if archPkg.digest != "" {
		t.Fatalf("x86_64 keyring is TLS-fetched and must stay unpinned, got %q", archPkg.digest)
	}

	for _, p := range vendorPackages {
		if p.source != nil {
			t.Fatalf("static vendor package %q must not override the source", p.name)
		}
		if got := p.resolvePkgSource().Arch(); got != "x86_64" {
			t.Fatalf("static vendor package %q must resolve via Arch Linux, got %s", p.name, got)
		}
	}
}

func TestVerifyPkgDigest_MatchAndMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pkg.tar.xz")
	os.WriteFile(p, []byte("stable content"), 0o644)

	sum := sha256.Sum256([]byte("stable content"))
	want := hex.EncodeToString(sum[:])
	if err := verifyPkgDigest(p, want); err != nil {
		t.Fatalf("matching digest rejected: %v", err)
	}
	err := verifyPkgDigest(p, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch error, got %v", err)
	}
}

// --- keyring extraction is confined to keyring files ---

func TestExtractKeyringPkg_CannotOverwriteExecutables(t *testing.T) {
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	mustTarEntry(t, tw, &tar.Header{Name: "usr/bin/pacman-key", Mode: 0o755, Size: 7}, "#!/bin/")
	mustTarEntry(t, tw, &tar.Header{Name: "usr/share/pacman/keyrings/alarm.gpg", Mode: 0o644, Size: 5}, "keys\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	pkgPath := filepath.Join(t.TempDir(), "keyring-1.0-1-any.pkg.tar.xz")
	pf, err := os.Create(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	xw, err := xz.NewWriter(pf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := xw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pf.Close(); err != nil {
		t.Fatal(err)
	}

	vendor := t.TempDir()
	if err := extractKeyringPkg(pkgPath, vendor); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(vendor, "usr", "bin")); !os.IsNotExist(err) {
		t.Fatal("keyring package must not create usr/bin in the vendor tree")
	}
	got := readFile(t, filepath.Join(vendor, "usr/share/pacman/keyrings/alarm.gpg"))
	if got != "keys\n" {
		t.Fatalf("keyring file: got %q", got)
	}
}

// --- truncated package sniffing ---

func TestOpenPkgDecompressor_TruncatedInput(t *testing.T) {
	for n := 0; n <= 5; n++ {
		p := filepath.Join(t.TempDir(), "pkg.tar.xz")
		os.WriteFile(p, bytes.Repeat([]byte{0xFF}, n), 0o644)
		_, closer, err := openPkgDecompressor(p)
		if err == nil {
			if closer != nil {
				closer()
			}
			t.Fatalf("input of %d bytes: expected error, got a reader", n)
		}
	}
}

// --- Collect-level arch validation ---

func writeCollectProject(t *testing.T, targetYAML string) *config.Project {
	t.Helper()
	dir := t.TempDir()
	projYAML := "name: archtest\ntargets:\n" + targetYAML
	if err := os.WriteFile(filepath.Join(dir, "starforge.yaml"), []byte(projYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "layers", "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	layer := "name: base\nsteps:\n  - action: pacman-add\n    packages:\n      - bash\n"
	if err := os.WriteFile(filepath.Join(dir, "layers", "base", "layer.yaml"), []byte(layer), 0o644); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestCollect_RejectsUnsupportedArch(t *testing.T) {
	proj := writeCollectProject(t, "  bad:\n    arch: armv7h\n    layers:\n      - layers/base\n")
	b := NewBuilder(proj)
	_, err := b.Collect(proj.Targets["bad"], false)
	if err == nil || !strings.Contains(err.Error(), `unsupported target arch "armv7h"`) {
		t.Fatalf("expected unsupported arch error, got %v", err)
	}
}

func TestCollect_RejectsPinnedOnArchARM(t *testing.T) {
	proj := writeCollectProject(t, "  arm:\n    arch: aarch64\n    layers:\n      - layers/base\n      - layers/pinned\n")
	// Replace the base layer with a pinned package declaration.
	pinned := "name: pinned\nsteps:\n  - action: pacman-add\n    packages:\n      - bash=5.2.037-2\n"
	layerDir := filepath.Join(proj.Dir, "layers", "pinned")
	if err := os.MkdirAll(layerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "layer.yaml"), []byte(pinned), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(proj)
	_, err := b.Collect(proj.Targets["arm"], false)
	if err == nil || !strings.Contains(err.Error(), "pinned packages are not supported on arch aarch64") {
		t.Fatalf("expected pinned-package rejection, got %v", err)
	}
}

func TestCollect_SetsNormalizedArch(t *testing.T) {
	proj := writeCollectProject(t, "  x86:\n    layers:\n      - layers/base\n  arm:\n    arch: aarch64\n    layers:\n      - layers/base\n")
	b := NewBuilder(proj)

	ctx, err := b.Collect(proj.Targets["x86"], false)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Arch != "x86_64" {
		t.Fatalf("default arch: got %q, want x86_64", ctx.Arch)
	}

	ctx, err = b.Collect(proj.Targets["arm"], false)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Arch != "aarch64" {
		t.Fatalf("arch: aarch64: got %q", ctx.Arch)
	}
}

// --- binfmt preflight ---

func TestRequireBinfmt(t *testing.T) {
	// Same-arch targets never require emulation.
	var hostPacmanArch string
	switch runtime.GOARCH {
	case "arm64":
		hostPacmanArch = "aarch64"
	default:
		hostPacmanArch = "x86_64"
	}
	if err := RequireBinfmt(hostPacmanArch); err != nil {
		t.Fatalf("same-arch target should not require binfmt: %v", err)
	}
	if err := RequireBinfmt("armv7h"); err == nil {
		t.Fatal("unknown arch must error")
	}
}

// --- binfmt registration parsing ---

func writeBinfmtEntry(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBinfmtHandlerRegisteredIn_RequiresEnabledAndFixBinary(t *testing.T) {
	cases := []struct {
		name    string
		entry   string
		content string
		want    bool
	}{
		{
			name:    "enabled qemu handler with F flag",
			entry:   "qemu-aarch64",
			content: "enabled\ninterpreter /usr/bin/qemu-aarch64\nflags: POCF\n",
			want:    true,
		},
		{
			name:    "host-specific name recognized",
			entry:   "aarch64",
			content: "enabled\ninterpreter /usr/bin/qemu-aarch64\nflags: POCF\n",
			want:    true,
		},
		{
			name:    "disabled registration rejected",
			entry:   "qemu-aarch64",
			content: "disabled\ninterpreter /usr/bin/qemu-aarch64\nflags: POCF\n",
			want:    false,
		},
		{
			name:    "missing fix_binary flag rejected",
			entry:   "qemu-aarch64",
			content: "enabled\ninterpreter /usr/bin/qemu-aarch64\nflags: OC\n",
			want:    false,
		},
		{
			name:    "unrelated arch rejected",
			entry:   "qemu-riscv64",
			content: "enabled\ninterpreter /usr/bin/qemu-riscv64\nflags: POCF\n",
			want:    false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeBinfmtEntry(t, dir, c.entry, c.content)
			if got := binfmtHandlerRegisteredIn(dir, "aarch64"); got != c.want {
				t.Fatalf("binfmtHandlerRegisteredIn = %v, want %v", got, c.want)
			}
		})
	}
}

func TestBinfmtHandlerRegisteredIn_MissingDirectory(t *testing.T) {
	if binfmtHandlerRegisteredIn(filepath.Join(t.TempDir(), "absent"), "aarch64") {
		t.Fatal("missing binfmt directory must report not registered")
	}
}

func TestRequireHostToolchain(t *testing.T) {
	err := RequireHostToolchain()
	if runtime.GOARCH == "amd64" {
		if err != nil {
			t.Fatalf("amd64 hosts run the vendored toolchain: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("non-amd64 host (%s) must fail loudly", runtime.GOARCH)
	}
}

// --- verified fetch: digest eviction and size cap ---

func TestFetchVendorPackage_EvictsPoisonedCacheEntry(t *testing.T) {
	good := []byte("good package bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(good)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "pkg.tar.xz")
	os.WriteFile(cachePath, []byte("poisoned"), 0o644)

	sum := sha256.Sum256(good)
	want := hex.EncodeToString(sum[:])

	// A poisoned cached file fails verification and is evicted.
	err := fetchVendorPackage(srv.URL, cachePath, want)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
	if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
		t.Fatal("poisoned cache entry must be evicted")
	}

	// The next run re-downloads and succeeds.
	if err := fetchVendorPackage(srv.URL, cachePath, want); err != nil {
		t.Fatalf("re-fetch after eviction failed: %v", err)
	}
	got, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, good) {
		t.Fatalf("cached bytes = %q", got)
	}
}

func TestDownloadFile_RejectsOversizedResponse(t *testing.T) {
	orig := maxPackageDownloadBytes
	maxPackageDownloadBytes = 1024
	defer func() { maxPackageDownloadBytes = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("A"), 4096))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "pkg.tar.xz")
	if err := downloadFile(srv.URL, dest); err == nil {
		t.Fatal("expected oversized download to fail")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("oversized download must not be cached")
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("staging file must be removed")
	}
}

func TestDownloadFile_StagesThenRenames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "pkg.tar.xz")
	if err := downloadFile(srv.URL, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("downloaded content = %q", got)
	}
}

// --- BuildResult carries the target arch ---

func TestBuildResult_RoundTripsArch(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Arch = "aarch64"

	mapped := contextToBuildResult(ctx)
	restored := buildResultToContext(&mapped)
	if restored.Arch != "aarch64" {
		t.Fatalf("arch lost in BuildResult mapping: got %q", restored.Arch)
	}

	dir := t.TempDir()
	if err := SaveBuildResult(ctx, dir); err != nil {
		t.Fatal(err)
	}
	result, err := LoadBuildResult(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := buildResultToContext(result).Arch; got != "aarch64" {
		t.Fatalf("arch lost in save/load round trip: got %q", got)
	}
}

func TestKeyringVendorPkg_PinnedURLDrift(t *testing.T) {
	alarmPkg := keyringVendorPkg(alarmSource{})
	if alarmPkg.url == "" {
		t.Fatal("ALARM keyring must pin its exact file URL")
	}
	if !strings.HasSuffix(alarmPkg.url, "/aarch64/core/archlinuxarm-keyring-20240419-2-any.pkg.tar.xz") {
		t.Fatalf("pinned keyring URL must match the digest-pinned file, got %q", alarmPkg.url)
	}

	archPkg := keyringVendorPkg(archLinuxSource{})
	if archPkg.url != "" {
		t.Fatalf("x86_64 keyring stays on dynamic resolution, got %q", archPkg.url)
	}
}
