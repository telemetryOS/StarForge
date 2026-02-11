# StarForge Documentation

StarForge is a declarative CLI tool for building custom Arch Linux OS images from layered recipes. You define your partitions, packages, users, services, files, and boot configuration in YAML, and StarForge assembles it all into bootable disk images with incremental caching.

## Quick Start

```bash
# Create a new project
starforge init my-os

# Enter the project directory
cd my-os

# Build the default target
starforge build distribution

# Test in QEMU
starforge run distribution

# Write to a USB drive
starforge write distribution /dev/sdX

# Export a full disk image
starforge export distribution disk --size 16G
```

See the [Getting Started Guide](guide.md) for a complete walkthrough. For YAML syntax details including custom tags and quoting rules, see the [YAML Reference](yaml-reference.md). For common issues, see [Troubleshooting](troubleshooting.md).

## Commands

| Command | Description |
|---------|-------------|
| [init](commands/init.md) | Create a new StarForge project |
| [build](commands/build.md) | Build disk images for a target |
| [write](commands/write.md) | Write a built target to a storage device |
| [chroot](commands/chroot.md) | Enter the built filesystem interactively |
| [run](commands/run.md) | Boot a built target in QEMU |
| [export](commands/export.md) | Export build artifacts as disk or partition images |
| [inspect](commands/inspect.md) | Inspect the resolved build context for a target |
| [list](commands/list.md) | List targets defined in the project |
| [status](commands/status.md) | Show project info and build state |
| [clean](commands/clean.md) | Remove build artifacts |

## Concepts

### Projects

A StarForge project is a directory containing a `starforge.yaml` file. This file defines the project name, an optional description, and one or more **targets**.

```yaml
name: Edge-OS
description: TelemetryOS Edge Player OS

targets:
  device:
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/player
```

### Targets

A target is a named build profile that specifies an ordered list of **layers**. Different targets can combine different layers to produce different OS variants from a shared base.

Layer entries can be local paths, git repository URLs, archive URLs, or HTTP(S) URLs pointing to a remote layer directory:

```yaml
targets:
  device:
    layers:
      - ./layers/base                                        # local path
      - https://github.com/org/shared-layer.git#v2.0        # git repo
      - https://example.com/resources-v2.tar.gz              # archive
      - https://example.com/layers/desktop/                  # remote layer
      - ./layers/app
```

- **Git repositories**: URLs ending in `.git`, with an optional `#branch` or `#tag` ref. Shallow cloned.
- **Archives**: `.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.zip` URLs. Extracted with the top-level directory stripped.
- **Remote layers**: Any other HTTP(S) URL. StarForge downloads `layer.yaml` from the URL and automatically fetches all files referenced by the layer's steps (`layer_path`, `script_path`, and `!include` paths). The remote URL is treated as the layer directory root -- relative paths in the layer are resolved against it.

Remote layers are fetched once and cached in `.starforge/cache/remote/`. Git and archive sources are cached in `.starforge/cache/sources/`.

### Layers

A layer is a directory containing a `layer.yaml` file. The directory can also contain any additional files or subdirectories referenced by actions (e.g., files to copy, scripts to run). Each layer defines a list of **steps** -- each step specifies an action and its configuration.

Steps can include an optional `label` for readability in build output and `starforge inspect`, and `layer_source` to pull files from a git repo or archive for that step. The `!include` tag lets you split large layers into multiple files. See the [Actions Reference](actions/README.md) for details on step fields and YAML tags, and the [YAML Reference](yaml-reference.md) for the complete tag and quoting guide.

Layers are processed in order. Later layers can override or extend earlier ones:
- **Replace semantics**: `systemd-boot-install`, `systemd-target` (set-default), `system-hostname`, `system-locale`, `system-timezone`, `system-keymap` -- the last layer's value wins.
- **Replace-on-path semantics**: `file-create` -- later layers replace earlier files at the same path.
- **Accumulate with replace-on-name**: `partition-add` -- partitions accumulate across layers; a later partition with the same name replaces the earlier definition in place.
- **Remove semantics**: `pacman-remove` -- removes matching items accumulated by earlier layers.
- **Accumulate semantics**: `pacman-add`, `system-group`, `systemd-service` (enable/disable/mask), `systemd-mount`, `systemd-timer`, `systemd-socket`, `systemd-slice`, `systemd-target` (unit management), `file-edit`, `file-copy`, `file-move`, `file-delete`, `file-link`, `file-permissions`, `file-ownership`, `file-mkdir`, `run` -- values from all layers are combined.
- **Merge-on-name**: `system-user` -- a later layer referencing the same user name modifies the existing user instead of creating a new one. Fields like `groups` support `!add` and `!remove` tags for fine-grained control.

### Actions

Actions are the building blocks of layers. Each step in a layer specifies an action and its configuration. See the [Actions Reference](actions/README.md) for full details on every action.

| Action | Description | Semantics |
|--------|-------------|-----------|
| `partition-add` | Define partitions (accumulate with replace-on-name) | Accumulate |
| `partition-remove` | Remove a partition by name | -- |
| `partition-change` | Modify partition fields by name | -- |
| `pacman-add` | Add pacman packages | Accumulate |
| `pacman-remove` | Remove pacman packages by name | Remove |
| `system-user` | Create or modify a user account | Merge-on-name |
| `system-group` | Create an explicit group | Accumulate |
| `systemd-service` | Manage systemd service units | Accumulate |
| `systemd-mount` | Manage systemd mount units | Accumulate |
| `systemd-timer` | Manage systemd timer units | Accumulate |
| `systemd-socket` | Manage systemd socket units | Accumulate |
| `systemd-slice` | Manage systemd slice units | Accumulate |
| `systemd-target` | Set default target or manage target units | Replace / Accumulate |
| `file-create` | Create files from layers or inline content | Replace-on-path |
| `file-edit` | Modify existing file content | Accumulate |
| `file-copy` | Copy files within the target filesystem | Accumulate |
| `file-move` | Move/rename files within the target | Accumulate |
| `file-delete` | Remove files or directories | Accumulate |
| `file-link` | Create symbolic or hard links | Accumulate |
| `file-permissions` | Set file mode (chmod) | Accumulate |
| `file-ownership` | Set file ownership | Accumulate |
| `file-mkdir` | Create directories | Accumulate |
| `system-hostname` | Set the system hostname | Replace |
| `system-locale` | Set the system locale and locale-gen list | Replace / Accumulate |
| `system-timezone` | Set the system timezone | Replace |
| `system-keymap` | Set the keyboard map | Replace |
| `systemd-boot-install` | Configure systemd-boot | Replace |
| `run` | Execute a script (file or inline) during build | Accumulate |

### Build Pipeline

StarForge builds in three stages:

1. **Collect** -- Process all layers in order, accumulating actions into a build context.
2. **Execute** -- Run 9 sequential phases via overlayfs, with per-phase caching:
   - `preinstall` -- Write vconsole.conf for keymap (before pacstrap)
   - `packages` -- Install packages via pacstrap
   - `sysconfig` -- Set hostname, locale, timezone, keymap
   - `users` -- Create groups and user accounts
   - `files` -- Create directories, copy files, create/edit files, create systemd unit files, copy/move/link/delete
   - `permissions` -- Set ownership (chown) and permissions (chmod)
   - `services` -- Enable/disable/mask systemd units, set default target
   - `boot` -- Configure systemd-boot loader and entries
   - `scripts` -- Execute build scripts in chroot
3. **Package** -- Copy the merged filesystem into partition images.

### Caching

Each build phase produces an overlayfs layer stored in `.starforge/<target>/cache/`. A SHA-256 hash of each phase's inputs is recorded in a manifest. On subsequent builds, unchanged phases are skipped entirely.

If a phase changes, all subsequent phases are invalidated (cascade invalidation). Use `--clean` with `starforge build` to force a full rebuild.

### Build Directory

All build artifacts are stored in `.starforge/` within the project root:

```
.starforge/<target>/
├── cache/
│   ├── manifest.json              # Phase hashes
│   ├── 0-preinstall/upper/        # Phase 0 delta
│   ├── 1-packages/upper/          # Phase 1 delta
│   ├── ...
│   └── 8-scripts/upper/           # Phase 8 delta
├── overlays/                      # Named overlays (from --overlay flag)
│   └── <name>/                    # Copies of partition images with persistent changes
├── merged/                        # Overlay mount point
├── boot.img                       # Partition images
├── root.img
└── ...
```

This directory should be added to `.gitignore` (done automatically by `starforge init`).

## Requirements

StarForge runs on any Linux system. Most build tools (pacstrap, pacman, mkfs, sgdisk, dmsetup, OVMF firmware, etc.) are **vendored automatically** -- downloaded from Arch Linux repositories on first use and cached in `~/.local/share/starforge/`.

Host requirements:

- **Linux** (any distribution) with overlayfs support (standard on all modern kernels)
- **Root access** (for overlayfs, chroot, and device operations)
- **Internet access** on first run to download vendored dependencies
- **QEMU** (`qemu-system-x86_64`) for `starforge run` -- this is the only tool that must be installed on the host

Vendored dependencies can be removed with `starforge clean deps` and will be re-downloaded on the next build.

## Reference

| Document | Contents |
|----------|----------|
| [Getting Started Guide](guide.md) | Complete walkthrough from project creation to deployment |
| [Actions Reference](actions/README.md) | All actions, step fields, override semantics, and build phases |
| [YAML Reference](yaml-reference.md) | Custom tags, quoting rules, INI field conversion, and common patterns |
| [Troubleshooting](troubleshooting.md) | Common issues and solutions |
