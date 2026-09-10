---
title: "Building & Testing"
weight: 15
---

This page covers the full build-test-debug cycle: building targets, understanding the build pipeline and caching, inspecting resolved configuration, and testing sealed artifacts in QEMU.

## Building

Build a target with the [`starforge build`](../../commands/build/) command:

```bash
starforge build distribution
```

**First build:** StarForge downloads and verifies an architecture-specific bootstrap, generates the managed builder appliance, and runs all 9 phases. Packages are installed via pacstrap, and all configuration phases execute in order.

**Subsequent builds:** StarForge uses overlayfs caching to skip phases whose inputs have not changed. Only modified phases and those that follow them are re-executed. A build where only a late phase (such as services or scripts) changed will skip the earlier phases entirely.

**Full rebuild:** Use the `--clean` flag to delete the cache and force all phases to run:

```bash
starforge build distribution --clean
```

## Build Pipeline Overview

StarForge builds in three stages:

1. **Collect** -- Process all layers in order, running each action to accumulate the desired state into a `BuildContext`. No filesystem changes happen during this stage. Variable substitution, `!include` resolution, and `layer_script` execution all occur here.

2. **Execute** -- Run the 9 sequential build phases. Each phase reads from the collected `BuildContext` and applies changes to the target rootfs using overlayfs layers. Phases are cached individually so unchanged phases are skipped.

3. **Package** -- Populate the declared GPT partitions in a full-disk image, then seal that artifact with its platform, layout, source hash, size, and SHA-256 digest. This stage always runs after Execute completes.

With the VM executor, privileged filesystem operations, mounts, and target chroots occur only inside the disposable builder VM. The host provides a read-only project disk, a persistent work disk, and a separate output disk. The appliance root itself is an ephemeral QCOW2 overlay.

## Build Phases

The Execute stage is divided into 9 phases that run in a fixed order:

| Phase | Name | What Runs |
|-------|------|-----------|
| 0 | `preinstall` | `system-keymap` (writes `vconsole.conf` before pacstrap so `mkinitcpio` picks it up) |
| 1 | `packages` | `pacman-add` (runs pacstrap with deduplicated package list, initializes pacman keyring) |
| 2 | `sysconfig` | `system-hostname`, `system-locale`, `system-timezone` (keymap already written in phase 0) |
| 3 | `users` | `system-group` then `system-user` |
| 4 | `files` | File operations in fixed sub-order: mkdir, layer copies, creates, edits, internal copies, moves, links, deletes. Systemd unit file creation also runs in this phase. |
| 5 | `permissions` | `file-ownership` (chown) then `file-permissions` (chmod) |
| 6 | `services` | mask, enable, disable, user-enable, user-disable, set default target |
| 7 | `boot` | `systemd-boot-install` (loader.conf and boot entries) |
| 8 | `scripts` | `run` actions via arch-chroot |

The sub-ordering within phases ensures correctness. For example, directories are created before files are copied into them (phase 4), and ownership is set before permissions (phase 5).

## Caching

Each phase produces an overlayfs upper directory that captures only the filesystem changes made during that phase. A SHA-256 hash of the phase's inputs is recorded in `manifest.json`. On subsequent builds, phases whose input hash matches the cached hash are skipped entirely.

### Cascade Invalidation

If phase N changes (its input hash differs from the cached hash), phases N through 8 are all invalidated. This ensures correctness -- later phases may depend on the output of earlier ones.

For example, adding a new package invalidates phase 1 (packages) and all subsequent phases. Changing only a file-create step invalidates phase 4 (files) and phases 5 through 8, but phases 0 through 3 are reused from cache.

### Cache Directory Layout

```
.starforge/<target>/cache/
  manifest.json                # Phase hashes and completion status
  0-preinstall/upper/          # Phase 0 filesystem delta
  1-packages/upper/            # Phase 1 filesystem delta
  2-sysconfig/upper/           # Phase 2 filesystem delta
  3-users/upper/               # Phase 3 filesystem delta
  4-files/upper/               # Phase 4 filesystem delta
  5-permissions/upper/         # Phase 5 filesystem delta
  6-services/upper/            # Phase 6 filesystem delta
  7-boot/upper/                # Phase 7 filesystem delta
  8-scripts/upper/             # Phase 8 filesystem delta
```

### When to Use `--clean`

Use `starforge build <target> --clean` when:

- The cache format has changed (StarForge detects version mismatches automatically, but `--clean` is a safe fallback)
- You suspect the cache is corrupted or out of sync
- You want a reproducible build from scratch for release purposes
- Debugging a build issue where caching might mask the problem

## Inspecting the Build

The [`starforge inspect`](../../commands/inspect/) command shows how your layers resolve without running a build. This is useful for verifying configuration before committing to a full build.

### Basic Usage

Show all resolved configuration for a target:

```bash
starforge inspect distribution
```

Show a specific concern:

```bash
starforge inspect distribution packages
starforge inspect distribution partitions
starforge inspect distribution services
```

### Available Concerns

| Concern | What It Shows |
|---------|---------------|
| `partitions` | Partition layout (name, filesystem, size, mount point, type) |
| `packages` | Deduplicated package list after adds and removes |
| `groups` | Explicitly defined groups |
| `users` | User accounts with merged group memberships |
| `services` | Enabled, disabled, and masked services; default target |
| `files` | All file operations (creates, copies, edits, moves, links, deletes, mkdirs) |
| `permissions` | Ownership and mode changes |
| `boot` | Bootloader configuration (loader.conf and entries) |
| `system` | Hostname, locale, timezone, keymap |
| `scripts` | Run action scripts |

### Layer Provenance

The `--layers` / `-l` flag shows which layer contributed each item. For fields with replace semantics (like hostname), it marks overridden values:

```bash
# See which layer set the hostname
starforge inspect distribution system -l

# See which layers contribute packages
starforge inspect distribution packages --layers

# Show partition layout with layer provenance
starforge inspect distribution partitions --layers
```

## Testing in QEMU

The [`starforge run`](../../commands/run/) command boots a built target in a QEMU virtual machine:

```bash
starforge run distribution
```

SSH into the VM from another terminal on port 2222:

```bash
ssh -p 2222 localhost
```

### Serial Console

Use `--serial` to attach a serial console for kernel boot messages and direct terminal access:

```bash
starforge run distribution --serial
```

### Disposable Runtime State

Every run uses QEMU's disposable snapshot mode:

```bash
starforge run distribution
```

The VM never modifies the sealed artifact. Changes made during the session are discarded when QEMU exits. Put a desired change into a layer and rebuild it so the result remains reproducible.

## Interactive Debugging

Use the target VM's serial console, graphical console, or SSH service to debug the actual booted system:

```bash
starforge run --serial distribution
ssh -p 2222 localhost
```

This tests the complete firmware, bootloader, kernel, initramfs, service, and runtime path. The public mount, unmount, and chroot workflow has been removed.

## Build Directory Structure

All build artifacts are stored in `.starforge/` within the project root. This directory is added to `.gitignore` by `starforge init`.

```
.starforge/
├── <target>/                    # Per-target build directory
│   ├── cache/                   # Overlay cache (one snapshot per phase)
│   │   ├── manifest.json        # Phase hashes for incremental builds
│   │   ├── 0-preinstall/upper/  # Phase 0 filesystem delta
│   │   ├── 1-packages/upper/    # Phase 1 filesystem delta
│   │   └── ...
│   ├── artifacts/
│   │   └── <arch>/<machine>/<distribution>/
│   │       ├── disk.raw         # Sealed full-disk artifact
│   │       └── artifact.json    # Platform, source hash, size, and digest
│   └── ...                       # Native executor working state, when used
├── vm/
│   └── <arch>/<distribution>/
│       └── work.raw              # Persistent builder package and phase cache
├── cache/
│   ├── sources/                 # Cached git repos and archives
│   ├── remote/                  # Cached remote HTTP layers
│   └── downloads/               # Cached individual file downloads
```

Downloaded builder-appliance inputs and generated immutable roots are stored in the user's StarForge cache, outside the project directory. Native-executor tools, when used, remain in StarForge's user data directory.

## See Also

- [Deploying](../deploying/) -- Writing to devices and exporting images.
- [Multi-Target Projects](../multi-target-projects/) -- Managing multiple build targets.
- [Remote Layers](../remote-layers/) -- Remote source caching behavior.
- [`starforge build`](../../commands/build/) -- Build command reference.
- [`starforge inspect`](../../commands/inspect/) -- Inspect command reference.
- [`starforge run`](../../commands/run/) -- Run command reference.
