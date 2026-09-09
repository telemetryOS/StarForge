package appliance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/telemetryos/starforge/platform"
)

const (
	archReleaseFingerprint    = "3E80CA1A8B89F69CBA57D98A76A5EF9054449A5C"
	archLinuxARMFingerprint   = "68B3537F39A313B3E574D06777193F152BDBE6A6"
	archKeyserver             = "hkps://keyserver.ubuntu.com"
	x86BootstrapURL           = "https://geo.mirror.pkgbuild.com/iso/latest/archlinux-bootstrap-x86_64.tar.zst"
	aarch64RootfsURL          = "https://ca.us.mirror.archlinuxarm.org/os/ArchLinuxARM-aarch64-latest.tar.gz"
	archLinuxPackageAPIFormat = "https://archlinux.org/packages/core/x86_64/%s/json/"
	archLinuxMirror           = "https://geo.mirror.pkgbuild.com/core/os/x86_64/"
)

type sourceArchive struct {
	Path        string
	StripPrefix string
	Metadata    bool
}

type packageInfo struct {
	Filename string `json:"filename"`
}

func (m *Manager) sources(ctx context.Context, targetPlatform platform.Spec, cacheDir string) ([]sourceArchive, error) {
	switch targetPlatform.Architecture {
	case platform.ArchitectureAArch64:
		rootfs, err := downloadVerified(ctx, cacheDir, aarch64RootfsURL, archLinuxARMFingerprint)
		if err != nil {
			return nil, err
		}
		return []sourceArchive{{Path: rootfs}}, nil
	case platform.ArchitectureX86_64:
		rootfs, err := downloadVerified(ctx, cacheDir, x86BootstrapURL, archReleaseFingerprint)
		if err != nil {
			return nil, err
		}
		linuxPackage, err := downloadArchPackage(ctx, cacheDir, "linux", rootfs)
		if err != nil {
			return nil, err
		}
		return []sourceArchive{
			{Path: rootfs, StripPrefix: "root.x86_64/"},
			{Path: linuxPackage, Metadata: true},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported appliance architecture %q", targetPlatform.Architecture)
	}
}

func downloadVerified(ctx context.Context, cacheDir, url, fingerprint string) (string, error) {
	archive, err := download(ctx, cacheDir, url)
	if err != nil {
		return "", err
	}
	signature, err := download(ctx, cacheDir, url+".sig")
	if err != nil {
		return "", err
	}
	if err := verifySignature(ctx, archive, signature, fingerprint); err != nil {
		_ = os.Remove(archive)
		_ = os.Remove(signature)
		return "", fmt.Errorf("verifying %s: %w", url, err)
	}
	return archive, nil
}

func downloadArchPackage(ctx context.Context, cacheDir, name, trustArchive string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(archLinuxPackageAPIFormat, name), nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("resolving Arch package %s: %w", name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving Arch package %s: HTTP %d", name, response.StatusCode)
	}
	var info packageInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("decoding Arch package %s: %w", name, err)
	}
	if info.Filename == "" {
		return "", fmt.Errorf("Arch package API returned incomplete metadata for %s", name)
	}
	url := archLinuxMirror + info.Filename
	path, err := download(ctx, cacheDir, url)
	if err != nil {
		return "", err
	}
	signature, err := download(ctx, cacheDir, url+".sig")
	if err != nil {
		return "", err
	}
	keyring := filepath.Join(cacheDir, "archlinux.gpg")
	if err := extractArchiveEntry(trustArchive, "root.x86_64/usr/share/pacman/keyrings/archlinux.gpg", keyring); err != nil {
		return "", fmt.Errorf("extracting signed Arch package keyring: %w", err)
	}
	if err := verifyKeyringSignature(ctx, path, signature, keyring); err != nil {
		_ = os.Remove(path)
		_ = os.Remove(signature)
		return "", fmt.Errorf("verifying Arch package %s: %w", name, err)
	}
	return path, nil
}

func download(ctx context.Context, cacheDir, url string) (string, error) {
	hash := sha256.Sum256([]byte(url))
	name := hex.EncodeToString(hash[:]) + "-" + filepath.Base(url)
	destination := filepath.Join(cacheDir, "downloads", name)
	if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
		return destination, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: HTTP %d", url, response.StatusCode)
	}
	temporary := destination + ".creating"
	file, err := os.Create(temporary)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("downloading %s: %w", url, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return "", closeErr
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	return destination, nil
}

func verifySignature(ctx context.Context, archive, signature, fingerprint string) error {
	home, err := os.MkdirTemp("", "starforge-gpg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	if err := os.Chmod(home, 0o700); err != nil {
		return err
	}
	receive := exec.CommandContext(ctx, "gpg", "--batch", "--homedir", home, "--keyserver", archKeyserver, "--recv-keys", fingerprint)
	if output, err := receive.CombinedOutput(); err != nil {
		return fmt.Errorf("retrieving signing key %s: %s: %w", fingerprint, strings.TrimSpace(string(output)), err)
	}
	verify := exec.CommandContext(ctx, "gpg", "--batch", "--homedir", home, "--status-fd", "1", "--verify", signature, archive)
	output, err := verify.CombinedOutput()
	if err != nil {
		return fmt.Errorf("detached signature: %s: %w", strings.TrimSpace(string(output)), err)
	}
	want := "[GNUPG:] VALIDSIG " + strings.ToUpper(fingerprint) + " "
	if !strings.Contains(strings.ToUpper(string(output)), want) {
		return fmt.Errorf("signature is not from expected key %s", fingerprint)
	}
	return nil
}

func verifyKeyringSignature(ctx context.Context, archive, signature, keyring string) error {
	home, err := os.MkdirTemp("", "starforge-gpg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	if err := os.Chmod(home, 0o700); err != nil {
		return err
	}
	importKeyring := exec.CommandContext(ctx, "gpg", "--batch", "--homedir", home, "--import", keyring)
	if output, err := importKeyring.CombinedOutput(); err != nil {
		return fmt.Errorf("importing signed package keyring: %s: %w", strings.TrimSpace(string(output)), err)
	}
	verify := exec.CommandContext(ctx, "gpg", "--batch", "--homedir", home, "--verify", signature, archive)
	if output, err := verify.CombinedOutput(); err != nil {
		return fmt.Errorf("detached signature: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
