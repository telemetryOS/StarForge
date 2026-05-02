---
title: "Troubleshooting"
weight: 9
---


Common issues and solutions when working with StarForge.

## Build Issues

### "overlay: upperdir is in-use as lowerdir" or stale mount errors

StarForge uses overlayfs extensively. If a previous build was interrupted (e.g., by Ctrl+C or a crash), stale mounts may remain.

**Solution**: StarForge automatically cleans up stale mounts at the start of each command. If mounts persist, run:

```bash
starforge clean <target>
```

This removes the cache and images, forcing a fresh build. If `clean` itself fails due to mounts, manually unmount:

```bash
# List StarForge-related mounts
mount | grep .starforge

# Unmount in reverse order (deepest first)
sudo umount /path/to/.starforge/<target>/merged
```

### Build fails during the packages phase (phase 1)

Pacstrap failures typically mean a package name is wrong or the network is unreachable.

**Check**:
- Verify package names: `pacman -Ss <package>` on an Arch system, or check the Arch package search at archlinux.org
- Ensure internet access — pacstrap downloads packages from Arch mirrors
- If behind a proxy, set `HTTP_PROXY` and `HTTPS_PROXY` environment variables before running `starforge build`

**If a package was removed from the repos**: Remove it from your `pacman-add` step and rebuild.

### Build fails during the scripts phase (phase 8)

Script errors are the most common build failure. The error output includes the script's stderr.

**Debug tips**:
- Use `starforge chroot <target>` to enter the filesystem and test commands interactively
- Add `set -ex` at the top of scripts for verbose output
- Check that commands are available — packages are installed in phase 1, so all binaries should be present by phase 8
- If a script needs network access, note that chroot environments have network access during build

### Cache appears corrupted

If builds produce unexpected results or errors reference missing files in the cache:

```bash
# Remove cache for a specific target
starforge clean <target> cache

# Or force a full rebuild
starforge build <target> --clean
```

The `--clean` flag ignores the cache and rebuilds all phases from scratch.

### "unknown action" error during YAML parsing

Action names are validated when the YAML is parsed. Check for typos in the `action` field:

```yaml
# WRONG
- action: pacman_add    # underscore instead of hyphen
- action: file_create   # underscore instead of hyphen
- action: systemd_service

# CORRECT
- action: pacman-add
- action: file-create
- action: systemd-service
```

### File mode interpreted as integer

If a file has unexpected permissions, the mode value may not be quoted:

```yaml
# WRONG — YAML interprets 0755 as the integer 493
mode: 0755

# CORRECT — quoted string preserved as "0755"
mode: "0755"
```

Always quote octal mode values. See the [YAML Reference](../yaml-reference/#octal-numbers--always-quote-file-modes) for details.

### "layer_script requires layer_source" error

The `layer_script` and `layer_script_path` fields on steps require `layer_source` to be set. These scripts run in the resolved source directory:

```yaml
# WRONG — no layer_source
- action: file-create
  layer_script: make build
  layer_path: ./output/app
  path: /usr/local/bin/app

# CORRECT
- action: file-create
  layer_source: https://github.com/org/app.git#v1.0
  layer_script: make build
  layer_path: ./output/app
  path: /usr/local/bin/app
```

## QEMU / `starforge run` Issues

### "qemu-system-x86_64: not found"

QEMU must be installed on the host system. It is the only dependency not vendored by StarForge.

```bash
# Arch Linux
sudo pacman -S qemu-full

# Ubuntu/Debian
sudo apt install qemu-system-x86

# Fedora
sudo dnf install qemu-system-x86
```

### VM boots to a shell but no network

Ensure NetworkManager or another network service is enabled in your layer:

```yaml
- action: systemd-service
  name: NetworkManager
  enable: true
```

The QEMU VM uses virtio networking. Most modern kernels include virtio drivers in the default configuration.

### VM boots but no display / black screen

StarForge uses virtio-vga for QEMU display. If your OS doesn't load the virtio-gpu driver, the display may be blank.

**Solutions**:
- Use `--serial` to get a serial console for debugging: `starforge run <target> --serial`
- Ensure the `mesa` package is installed for GPU support
- Check boot logs via serial for kernel panic or init errors

### SSH connection refused on port 2222

StarForge forwards host port 2222 to guest port 22. Check:
- Is `sshd` enabled? Add `enable: true` to your systemd-service step for `sshd`
- Is the `openssh` package installed?
- Has the VM finished booting? Wait for the login prompt before trying SSH
- Is another process using port 2222?

```bash
ssh -p 2222 -o StrictHostKeyChecking=no localhost
```

## Write / Export Issues

### "not a block device" error on `starforge write`

The device path must point to a block device (e.g., `/dev/sdb`, `/dev/mmcblk0`), not a partition (e.g., `/dev/sdb1`):

```bash
# WRONG — partition, not whole device
starforge write distribution /dev/sdb1

# CORRECT — whole device
starforge write distribution /dev/sdb
```

## Named Overlay Issues

### Overlay recreated unexpectedly

Named overlays are tied to the build hash. When you run `starforge build`, if any phase changes, existing overlays are invalidated and recreated on next use.

This is intentional — overlays must stay consistent with the current build. If you need to preserve overlay state, avoid rebuilding until you're done testing.

### Overlay names rejected

Overlay names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`:
- Start with a letter or digit
- Contain only letters, digits, hyphens, and underscores
- No spaces or special characters

```bash
# VALID
starforge chroot distribution --overlay testing
starforge chroot distribution --overlay my-test-1

# INVALID
starforge chroot distribution --overlay "my test"
starforge chroot distribution --overlay .hidden
```

## Remote Layer Issues

### Remote layer fetch fails

Remote layers require the server to respond with HTTP 200 for both `layer.yaml` and all referenced files. Check:

- Is the URL accessible? Try `curl <url>/layer.yaml`
- Does the remote server support directory listing or direct file access?
- Are all `layer_path` and `script_path` references valid paths relative to the layer URL?

Remote layers do not support authentication — URLs must be publicly accessible or use token-based query parameters if your server supports them.

### Files missing from remote layer

StarForge only fetches files explicitly referenced by steps (`layer_path`, `script_path`) and `!include` paths. It does not recursively download directories. If a step uses `layer_path` pointing to a directory, this will fail for remote layers — use a git or archive source instead for directory trees.

## General Tips

### Inspect before building

Use `starforge inspect` to verify your layer resolution without running a build:

```bash
# See everything
starforge inspect <target>

# See specific concerns
starforge inspect <target> packages
starforge inspect <target> services --layers
starforge inspect <target> system -l
```

The `--layers` / `-l` flag shows which layer contributed each item, with overridden values marked.

### Clean up vendored dependencies

If vendored tools seem corrupted or you need to update them:

```bash
starforge clean deps
```

This removes `~/.local/share/starforge/`. Dependencies are re-downloaded on the next build.

### Check build status

```bash
starforge status
```

Shows whether each target has been built and lists its layers.
