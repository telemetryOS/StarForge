package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/installer"
)

// bundleInstaller copies installer payloads, the server binary, and the
// client binary into the root partition image. This runs after PackageToImages
// so the partition images already exist and can be loop-mounted for writing.
func (b *Builder) bundleInstaller(ctx *actions.BuildContext, buildDir string) error {
	if !HasInstallerActions(ctx) {
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s\n", phaseStyle.Render("Bundling installer"))

	// Find the root partition image to write into
	rootImg := filepath.Join(buildDir, "root.img")
	if _, err := os.Stat(rootImg); err != nil {
		return fmt.Errorf("root partition image not found (needed for installer bundling): %w", err)
	}

	// Loop-mount the root partition
	rootfs, err := os.MkdirTemp("", "starforge-installer-*")
	if err != nil {
		return fmt.Errorf("creating temp mount dir: %w", err)
	}
	defer os.RemoveAll(rootfs)

	if err := run("mount", "-o", "loop", rootImg, rootfs); err != nil {
		return fmt.Errorf("mounting root image: %w", err)
	}
	defer run("umount", rootfs)

	return b.BundleInstallerToRootfs(ctx, rootfs)
}

// HasInstallerActions returns true if the build context includes any
// installer actions (payloads, server, or client).
func HasInstallerActions(ctx *actions.BuildContext) bool {
	return len(ctx.InstallerPayloads) > 0 || ctx.InstallerServer != nil || ctx.InstallerClient != nil
}

// BundleInstallerToRootfs copies installer payloads, the server binary, and
// the client binary into an already-mounted rootfs. Use this when the target
// filesystem is mounted at rootfs (e.g. during write-to-device or export).
func (b *Builder) BundleInstallerToRootfs(ctx *actions.BuildContext, rootfs string) error {
	// Bundle payloads
	if len(ctx.InstallerPayloads) > 0 {
		if err := b.bundlePayloads(ctx, rootfs); err != nil {
			return err
		}
	}

	// Bundle server binary and service
	if ctx.InstallerServer != nil {
		if err := bundleServer(ctx.InstallerServer, rootfs); err != nil {
			return err
		}
	}

	// Bundle client binary and autologin
	if ctx.InstallerClient != nil {
		if err := bundleClient(ctx.InstallerClient, ctx.InstallerServer, rootfs); err != nil {
			return err
		}
	}

	return nil
}

// bundlePayloads ensures each payload target is packaged, then copies
// its partition images (zstd-compressed) and manifest into the installer rootfs.
func (b *Builder) bundlePayloads(ctx *actions.BuildContext, rootfs string) error {
	for _, payload := range ctx.InstallerPayloads {
		fmt.Printf("    payload: %s\n", payload.Target)

		// Ensure the payload target is built and packaged — auto-builds
		// if needed, then guarantees all .img files exist.
		payloadCtx, err := b.EnsureBuiltAndPackaged(payload.Target)
		if err != nil {
			return fmt.Errorf("payload target %q: %w", payload.Target, err)
		}

		payloadBuildDir := b.project.TargetBuildDir(payload.Target)

		// Create payload directory in rootfs
		payloadDir := filepath.Join(rootfs, payload.Path)
		if err := os.MkdirAll(payloadDir, 0o755); err != nil {
			return fmt.Errorf("creating payload dir: %w", err)
		}

		// Build manifest from payload's partition defs
		manifest := installer.PayloadManifest{
			Name:        payload.Target,
			Description: payload.Label,
		}

		for _, part := range payloadCtx.Partitions {
			srcFile := fmt.Sprintf("%s.img", part.Name)
			srcImg := filepath.Join(payloadBuildDir, srcFile)

			zstFile := fmt.Sprintf("%s.img.zst", part.Name)
			destImg := filepath.Join(payloadDir, zstFile)
			fmt.Printf("      %s → %s (%s)\n", srcFile, zstFile, actions.FormatSize(part.Size))
			if err := run("zstd", "-T0", "-9", "-f", srcImg, "-o", destImg); err != nil {
				return fmt.Errorf("compressing partition image %s: %w", srcFile, err)
			}

			pp := installer.PayloadPartition{
				Name:       part.Name,
				Filesystem: part.Filesystem,
				Size:       part.Size,
				MountPoint: part.MountPoint,
				Type:       part.Type,
				Grow:       part.Grow,
				Image:      zstFile,
			}

			manifest.Partitions = append(manifest.Partitions, pp)
		}

		// Write manifest.json
		manifestData, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling manifest: %w", err)
		}
		if err := os.WriteFile(filepath.Join(payloadDir, "manifest.json"), manifestData, 0o644); err != nil {
			return fmt.Errorf("writing manifest: %w", err)
		}
	}

	return nil
}

// bundleServer copies the starforge-install-server binary into the rootfs
// and creates a systemd service to start it at boot.
func bundleServer(server *actions.InstallerServerDef, rootfs string) error {
	fmt.Printf("    server (port %d)\n", server.Port)

	// Find the server binary — look next to the running starforge binary first
	serverBin, err := buildCompanionBinary("starforge-install-server")
	if err != nil {
		return fmt.Errorf("locating starforge-install-server binary: %w", err)
	}

	// Copy binary
	destBin := filepath.Join(rootfs, "usr/bin/starforge-install-server")
	if err := os.MkdirAll(filepath.Dir(destBin), 0o755); err != nil {
		return err
	}
	if err := CopyFile(serverBin, destBin); err != nil {
		return fmt.Errorf("copying server binary: %w", err)
	}
	if err := os.Chmod(destBin, 0o755); err != nil {
		return err
	}

	// Create systemd service
	unitContent := fmt.Sprintf(`[Unit]
Description=StarForge Installer Daemon

[Service]
Type=simple
ExecStart=/usr/bin/starforge-install-server --port %d --payload-dir %s

[Install]
WantedBy=multi-user.target
`, server.Port, server.Path)

	unitPath := filepath.Join(rootfs, "etc/systemd/system/starforge-install-server.service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(unitContent), 0o644); err != nil {
		return fmt.Errorf("writing server unit: %w", err)
	}

	// Enable the service by creating the symlink
	wantsDir := filepath.Join(rootfs, "etc/systemd/system/multi-user.target.wants")
	if err := os.MkdirAll(wantsDir, 0o755); err != nil {
		return err
	}
	linkPath := filepath.Join(wantsDir, "starforge-install-server.service")
	os.Remove(linkPath)
	if err := os.Symlink("/etc/systemd/system/starforge-install-server.service", linkPath); err != nil {
		return fmt.Errorf("enabling server service: %w", err)
	}

	return nil
}

// bundleClient copies the starforge-install binary into the rootfs and
// creates an autologin getty override to run it on the configured tty.
func bundleClient(client *actions.InstallerClientDef, server *actions.InstallerServerDef, rootfs string) error {
	fmt.Printf("    client (auto_login: %s)\n", client.AutoLogin)

	// Find the client binary
	clientBin, err := buildCompanionBinary("starforge-install")
	if err != nil {
		return fmt.Errorf("locating starforge-install binary: %w", err)
	}

	// Copy binary
	destBin := filepath.Join(rootfs, "usr/bin/starforge-install")
	if err := os.MkdirAll(filepath.Dir(destBin), 0o755); err != nil {
		return err
	}
	if err := CopyFile(clientBin, destBin); err != nil {
		return fmt.Errorf("copying client binary: %w", err)
	}
	if err := os.Chmod(destBin, 0o755); err != nil {
		return err
	}

	// Create autologin getty override
	dropinDir := filepath.Join(rootfs, fmt.Sprintf(
		"etc/systemd/system/getty@%s.service.d", client.AutoLogin))
	if err := os.MkdirAll(dropinDir, 0o755); err != nil {
		return err
	}

	dropinContent := `[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
`

	if err := os.WriteFile(filepath.Join(dropinDir, "autologin.conf"), []byte(dropinContent), 0o644); err != nil {
		return fmt.Errorf("writing autologin drop-in: %w", err)
	}

	// Create root's .bash_profile to exec the TUI on login
	bashProfileDir := filepath.Join(rootfs, "root")
	if err := os.MkdirAll(bashProfileDir, 0o700); err != nil {
		return err
	}

	// Only run the TUI on the configured tty, not on serial or SSH
	port := 8100
	if server != nil {
		port = server.Port
	}
	args := fmt.Sprintf("--server http://localhost:%d", port)
	if client.Unattended {
		args += " --unattended"
	}
	bashProfile := fmt.Sprintf(`# StarForge installer auto-start
if [ "$(tty)" = "/dev/%s" ]; then
    exec /usr/bin/starforge-install %s
fi
`, client.AutoLogin, args)

	profilePath := filepath.Join(bashProfileDir, ".bash_profile")
	if err := os.WriteFile(profilePath, []byte(bashProfile), 0o644); err != nil {
		return fmt.Errorf("writing .bash_profile: %w", err)
	}

	return nil
}

// buildCompanionBinary builds a companion binary from source so it is always
// up to date with the current starforge code. The output is placed next to the
// running starforge binary.
func buildCompanionBinary(name string) (string, error) {
	exe, _ := os.Executable()
	binDir := filepath.Dir(exe)
	output := filepath.Join(binDir, name)

	modRoot := findModuleRoot()
	if modRoot == "" {
		return "", fmt.Errorf("cannot build %s: module root not found", name)
	}

	fmt.Printf("    building %s...\n", name)
	if err := run("go", "build", "-C", modRoot, "-o", output, "./cmd/"+name+"/"); err != nil {
		return "", fmt.Errorf("building %s: %w", name, err)
	}

	return output, nil
}

// findModuleRoot locates the source directory of the starforge module by
// reading the running binary's build info and searching likely locations.
func findModuleRoot() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	modulePath := info.Main.Path // e.g. "github.com/telemetryos/starforge"
	if modulePath == "" {
		return ""
	}

	// Check GOPATH/src/<module>
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	if dir := filepath.Join(gopath, "src", modulePath); isModuleRoot(dir, modulePath) {
		return dir
	}

	// Walk up from executable directory
	if exe, err := os.Executable(); err == nil {
		if dir := walkUpForModule(filepath.Dir(exe), modulePath); dir != "" {
			return dir
		}
	}

	// Walk up from CWD, also checking sibling directories at each level
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for {
			if isModuleRoot(dir, modulePath) {
				return dir
			}
			// Check siblings
			if entries, err := os.ReadDir(dir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						sibling := filepath.Join(dir, e.Name())
						if isModuleRoot(sibling, modulePath) {
							return sibling
						}
					}
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return ""
}

func walkUpForModule(dir, modulePath string) string {
	for {
		if isModuleRoot(dir, modulePath) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isModuleRoot(dir, modulePath string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	// Check first line: "module <path>"
	for _, line := range strings.SplitN(string(data), "\n", 5) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")) == modulePath
		}
	}
	return false
}

