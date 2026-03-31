package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

	out.Blank()
	out.Phase("Bundling installer")

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
	defer run("umount", "-R", rootfs)

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

	// Copy the starforge binary once if server or client needs it
	if ctx.InstallerServer != nil || ctx.InstallerClient != nil {
		if err := bundleStarforgeBinary(rootfs); err != nil {
			return err
		}
	}

	// Bundle server systemd service
	if ctx.InstallerServer != nil {
		if err := bundleServer(ctx.InstallerServer, rootfs); err != nil {
			return err
		}
	}

	// Bundle client autologin
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
		out.Info("payload: %s", payload.Target)

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
		var efiLabel string
		if ctx.InstallerServer != nil {
			efiLabel = ctx.InstallerServer.EFILabel
		}
		manifest := installer.PayloadManifest{
			Name:        payload.Target,
			Description: payload.Label,
			EFILabel:    efiLabel,
		}

		for _, part := range payloadCtx.Partitions {
			srcFile := fmt.Sprintf("%s.img", part.Name)
			srcImg := filepath.Join(payloadBuildDir, srcFile)

			zstFile := fmt.Sprintf("%s.img.zst", part.Name)
			destImg := filepath.Join(payloadDir, zstFile)
			if err := out.RunWithSpinner(fmt.Sprintf("%s → %s (%s)", srcFile, zstFile, actions.FormatSize(part.Size)), func() error {
				return runSilent("zstd", "-T0", "-9", "-f", srcImg, "-o", destImg)
			}); err != nil {
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

// bundleStarforgeBinary copies the running starforge binary into the rootfs
// at /usr/bin/starforge. Both the server and client subcommands are embedded
// in the main binary (busybox-style).
func bundleStarforgeBinary(rootfs string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating running starforge binary: %w", err)
	}

	destBin := filepath.Join(rootfs, "usr/bin/starforge")
	if err := os.MkdirAll(filepath.Dir(destBin), 0o755); err != nil {
		return err
	}
	if err := CopyFile(exe, destBin); err != nil {
		return fmt.Errorf("copying starforge binary: %w", err)
	}
	if err := os.Chmod(destBin, 0o755); err != nil {
		return err
	}

	return nil
}

// bundleServer creates a systemd service that runs `starforge install-server`
// at boot.
func bundleServer(server *actions.InstallerServerDef, rootfs string) error {
	out.Info("server (port %d)", server.Port)

	// Create systemd service
	unitContent := fmt.Sprintf(`[Unit]
Description=StarForge Installer Daemon

[Service]
Type=simple
ExecStart=/usr/bin/starforge install-server --port %d --payload-dir %s

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

// bundleClient creates an autologin getty override and a .bash_profile that
// execs `starforge install` on the configured tty.
func bundleClient(client *actions.InstallerClientDef, server *actions.InstallerServerDef, rootfs string) error {
	out.Info("client (auto_login: %s)", client.AutoLogin)

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
    exec /usr/bin/starforge install %s
fi
`, client.AutoLogin, args)

	profilePath := filepath.Join(bashProfileDir, ".bash_profile")
	if err := os.WriteFile(profilePath, []byte(bashProfile), 0o644); err != nil {
		return fmt.Errorf("writing .bash_profile: %w", err)
	}

	return nil
}
