package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/installer"
)

// HasInstallerActions returns true if the build context includes any
// installer actions (payloads, server, or client).
func HasInstallerActions(ctx *actions.BuildContext) bool {
	return len(ctx.InstallPayloads) > 0 || ctx.InstallServer != nil || ctx.InstallClient != nil
}

// BundleInstallerToRootfs copies installer payloads, the server binary, and
// the client binary into an already-mounted rootfs. Use this when the target
// filesystem is mounted at rootfs (e.g. during write-to-device or export).
func (b *Builder) BundleInstallerToRootfs(ctx *actions.BuildContext, rootfs string) error {
	// Bundle payloads
	if len(ctx.InstallPayloads) > 0 {
		if err := b.bundlePayloads(ctx, rootfs); err != nil {
			return err
		}
	}

	// Copy the starforge binary once if server or client needs it
	if ctx.InstallServer != nil || ctx.InstallClient != nil {
		if err := bundleStarforgeBinary(rootfs); err != nil {
			return err
		}
	}

	if ctx.InstallServer != nil {
		if err := bundleBmaptool(rootfs); err != nil {
			return err
		}
	}

	// Bundle server systemd service
	if ctx.InstallServer != nil {
		if err := bundleServer(ctx.InstallServer, rootfs); err != nil {
			return err
		}
	}

	// Bundle client autologin
	if ctx.InstallClient != nil {
		if err := bundleClient(ctx.InstallClient, ctx.InstallServer, rootfs); err != nil {
			return err
		}
	}

	return nil
}

// bundlePayloads compresses each payload target's partition images (filtered
// by the payload's optional Partitions list) and copies them into rootfs at
// the configured path along with a manifest.json.
//
// Most callers reach this from packageOneTarget, where the payload target's
// images already exist on disk because of build ordering (host first, then
// embeds; non-cycle payloads built+packaged upfront via buildRecursive). If
// images aren't found, we fall back to EnsurePackaged for the payload target
// — this handles the install-payload-of-an-unembedded-target case (e.g. the
// installer target bundling device).
func (b *Builder) bundlePayloads(ctx *actions.BuildContext, rootfs string) error {
	for _, payload := range ctx.InstallPayloads {
		out.Info("payload: %s", payload.Target)

		payloadBuildDir := b.project.TargetBuildDir(payload.Target)

		// Ensure the payload target's images match its current build result
		// before bundling. Existing image files may be stale after a layout
		// or fstab-generation change, so existence alone is not enough.
		payloadResult, err := LoadBuildResult(payloadBuildDir)
		if err != nil {
			ctxOut, perr := b.EnsurePackaged(payload.Target)
			if perr != nil {
				return fmt.Errorf("payload target %q: %w", payload.Target, perr)
			}
			payloadResult = &BuildResult{Partitions: ctxOut.Partitions}
		} else if b.packaging[payload.Target] {
			out.Warning(fmt.Sprintf("payload %q: target is already being packaged on this stack — using in-progress images", payload.Target))
		} else {
			ctxOut, perr := b.EnsurePackaged(payload.Target)
			if perr != nil {
				return fmt.Errorf("packaging payload target %q: %w", payload.Target, perr)
			}
			payloadResult = &BuildResult{Partitions: ctxOut.Partitions}
		}

		// Apply the partitions filter, if any. Empty list means "all".
		parts := filterPartitions(payloadResult.Partitions, payload.Partitions)
		if len(payload.Partitions) > 0 && len(parts) == 0 {
			return fmt.Errorf("payload target %q: none of partitions %v match the target's layout", payload.Target, payload.Partitions)
		}

		// Materialize images on demand if any are missing. Self-payload
		// and payload-cycle re-entry: if EnsurePackaged is already running
		// for this target on the current stack, skip with a clear warning
		// rather than recursing — the in-progress call will produce the
		// images shortly and a subsequent run picks them up.
		if missing := firstMissingImage(payloadBuildDir, parts); missing != "" {
			if b.packaging[payload.Target] {
				out.Warning(fmt.Sprintf("payload %q: image %s missing and target is already being packaged on this stack — skipping bundle (will be valid on next run)", payload.Target, missing))
				continue
			}
			if _, perr := b.EnsurePackaged(payload.Target); perr != nil {
				return fmt.Errorf("packaging payload target %q (missing image %s): %w", payload.Target, missing, perr)
			}
		}

		// Create payload directory in rootfs
		payloadDir := filepath.Join(rootfs, payload.Path)
		if err := os.MkdirAll(payloadDir, 0o755); err != nil {
			return fmt.Errorf("creating payload dir: %w", err)
		}

		// Build manifest from the (filtered) partition defs.
		var efiLabel string
		if ctx.InstallServer != nil {
			efiLabel = ctx.InstallServer.EFILabel
		}
		manifest := installer.PayloadManifest{
			Name:        payload.Target,
			Description: payload.Label,
			EFILabel:    efiLabel,
		}

		for _, part := range parts {
			srcFile := fmt.Sprintf("%s.img", part.Name)
			srcImg := filepath.Join(payloadBuildDir, srcFile)
			srcBmap := srcImg + ".bmap"
			if err := ensureBmap(srcImg, srcBmap); err != nil {
				return fmt.Errorf("creating bmap for partition image %s: %w", srcFile, err)
			}

			zstFile := fmt.Sprintf("%s.img.zst", part.Name)
			destImg := filepath.Join(payloadDir, zstFile)
			if err := out.RunWithSpinner(fmt.Sprintf("%s → %s (%s)", srcFile, zstFile, actions.FormatSize(part.Size)), func() error {
				return runSilent("zstd", "-T0", "-9", "-f", srcImg, "-o", destImg)
			}); err != nil {
				return fmt.Errorf("compressing partition image %s: %w", srcFile, err)
			}
			bmapFile := fmt.Sprintf("%s.img.bmap", part.Name)
			destBmap := filepath.Join(payloadDir, bmapFile)
			if err := CopyFile(srcBmap, destBmap); err != nil {
				return fmt.Errorf("copying bmap %s: %w", bmapFile, err)
			}

			pp := installer.PayloadPartition{
				Name:       part.Name,
				Filesystem: part.Filesystem,
				Size:       part.Size,
				MountPoint: part.MountPoint,
				Type:       part.Type,
				Grow:       part.Grow,
				Image:      zstFile,
				Bmap:       bmapFile,
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

// filterPartitions returns parts whose Name matches any entry in want. Empty
// want returns all parts. Order follows parts (the partition layout order),
// not want.
func filterPartitions(parts []actions.PartitionDef, want []string) []actions.PartitionDef {
	if len(want) == 0 {
		return parts
	}
	wantSet := make(map[string]bool, len(want))
	for _, n := range want {
		wantSet[n] = true
	}
	var out []actions.PartitionDef
	for _, p := range parts {
		if wantSet[p.Name] {
			out = append(out, p)
		}
	}
	return out
}

// firstMissingImage returns the name of the first partition image that
// doesn't exist in buildDir, or "" if all are present.
func firstMissingImage(buildDir string, parts []actions.PartitionDef) string {
	for _, p := range parts {
		img := filepath.Join(buildDir, p.Name+".img")
		if _, err := os.Stat(img); err != nil {
			return p.Name + ".img"
		}
	}
	return ""
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

func bundleBmaptool(rootfs string) error {
	vendorRoot := VendorDir()
	srcDir := filepath.Join(vendorRoot, "usr", "lib", "starforge", "bmaptool")
	destDir := filepath.Join(rootfs, "usr", "lib", "starforge", "bmaptool")
	if err := copyTree(srcDir, destDir, true); err != nil {
		return fmt.Errorf("copying bmaptool: %w", err)
	}

	wrapper := `#!/usr/bin/python
import runpy
import sys
sys.path.insert(0, "/usr/lib/starforge/bmaptool/src")
runpy.run_module("bmaptool", run_name="__main__")
`
	destBin := filepath.Join(rootfs, "usr", "bin", "bmaptool")
	if err := os.MkdirAll(filepath.Dir(destBin), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destBin, []byte(wrapper), 0o755)
}

// bundleServer creates a systemd service that runs `starforge install-server`
// at boot.
func bundleServer(server *actions.InstallServerDef, rootfs string) error {
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
func bundleClient(client *actions.InstallClientDef, server *actions.InstallServerDef, rootfs string) error {
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
