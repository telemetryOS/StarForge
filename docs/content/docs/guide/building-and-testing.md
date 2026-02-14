---
title: "Building & Testing"
weight: 15
---

This page covers the full build-test-debug cycle: building targets, understanding the build pipeline and caching, inspecting resolved configuration, testing in QEMU, and interactive debugging with chroot.

## Building

Build a target with the [`starforge build`](../../commands/build/) command:

```bash
starforge build distribution
```

**First build:** All 9 phases run in sequence. Vendored dependencies (pacstrap, mkfs, etc.) are downloaded on first use and cached in `~/.local/share/starforge/`. Packages are installed via pacstrap, and all configuration phases execute in order.

**Subsequent builds:** StarForge uses overlayfs caching to skip phases whose inputs have not changed. Only modified phases and those that follow them are re-executed. A build where only a late phase (such as services or scripts) changed will skip the earlier phases entirely.

**Full rebuild:** Use the `--clean` flag to delete the cache and force all phases to run:

```bash
starforge build distribution --clean
```

## Build Pipeline Overview

StarForge builds in three stages:

1. **Collect** -- Process all layers in order, running each action to accumulate the desired state into a `BuildContext`. No filesystem changes happen during this stage. Variable substitution, `!include` resolution, and `layer_script` execution all occur here.

2. **Execute** -- Run the 9 sequential build phases. Each phase reads from the collected `BuildContext` and applies changes to the target rootfs using overlayfs layers. Phases are cached individually so unchanged phases are skipped.

3. **Package** -- Mount all overlayfs layers as a read-only merged view and copy the filesystem into partition images (e.g., `boot.img`, `root.img`). This stage always runs after Execute completes.

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

### Persistent Changes with Overlays

Use `--overlay` to persist changes made inside the VM across reboots:

```bash
starforge run distribution --overlay testing
```

The overlay creates a copy of the partition images in `.starforge/<target>/overlays/testing/`. Changes made inside the VM are written to these copies, preserving the original build output.

## Interactive Debugging with Chroot

The [`starforge chroot`](../../commands/chroot/) command enters the built filesystem in an arch-chroot environment for interactive debugging:

```bash
starforge chroot distribution
```

This mounts the overlayfs layers and drops you into a root shell inside the target filesystem. You can inspect files, run commands, and test configuration.

### Chroot with Overlay

Use `--overlay` to make persistent changes that survive across chroot sessions:

```bash
starforge chroot distribution --overlay debug
```

Without `--overlay`, changes are discarded when the chroot exits.

### Running Specific Commands

Pass a command to execute it directly instead of entering an interactive shell:

```bash
starforge chroot distribution -- pacman -Q
starforge chroot distribution -- systemctl list-unit-files
```

## Named Overlays

Named overlays provide persistent writable copies of the build output. They are used by both `starforge run --overlay` and `starforge chroot --overlay`.

Overlays are stored in `.starforge/<target>/overlays/<name>/` as copies of the partition images. Multiple named overlays can coexist for different testing scenarios:

```bash
# Create separate overlays for different test scenarios
starforge run distribution --overlay baseline
starforge run distribution --overlay with-updates
```

**Invalidation:** When the underlying build changes (a new `starforge build` runs), named overlays are automatically invalidated and recreated from the fresh partition images on next use.

**Naming rules:** Overlay names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` (alphanumeric, hyphens, and underscores; must start with an alphanumeric character).

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
│   ├── overlays/                # Named overlays (from --overlay flag)
│   │   └── <name>/             # Copies of partition images with persistent changes
│   ├── boot.img                 # Partition images (build output)
│   ├── root.img
│   └── ...
├── cache/
│   ├── sources/                 # Cached git repos and archives
│   ├── remote/                  # Cached remote HTTP layers
│   └── downloads/               # Cached individual file downloads
```

Vendored build tools (pacstrap, mkfs, etc.) are stored separately in `~/.local/share/starforge/`, not inside the project directory.

## See Also

- [Deploying](../deploying/) -- Writing to devices and exporting images.
- [Multi-Target Projects](../multi-target-projects/) -- Managing multiple build targets.
- [Remote Layers](../remote-layers/) -- Remote source caching behavior.
- [`starforge build`](../../commands/build/) -- Build command reference.
- [`starforge inspect`](../../commands/inspect/) -- Inspect command reference.
- [`starforge run`](../../commands/run/) -- Run command reference.
- [`starforge chroot`](../../commands/chroot/) -- Chroot command reference.
