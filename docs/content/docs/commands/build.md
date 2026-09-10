---
title: "starforge build"
weight: 1
---


Build and seal an OS image for a target.

## Synopsis

```
starforge [--executor=vm|native|auto] build <target> [--clean]
```

## Description

Resolve layers for a target, execute the build phases, and seal one full-disk raw artifact. Artifacts are scoped by target, architecture, machine, and distribution under `.starforge/`.

The build process follows three stages:

1. **Collect** -- Process all layers and validate the configuration.
2. **Execute** -- Run 9 build phases via overlayfs with per-phase caching.
3. **Package** -- Create the GPT disk, populate its partitions, and seal the full-disk artifact with a receipt and SHA-256 digest.

Phases are cached using overlayfs -- unchanged phases are skipped on subsequent builds. Use `--clean` to force a full rebuild.

The default `--executor=vm` always uses a managed QEMU builder appliance,
regardless of host privileges. This supports same-architecture and
cross-architecture builds without host mounts, loop devices, or root access.
Use `--executor=native` explicitly to build on the host. The compatibility mode
`--executor=auto` may choose native execution only when StarForge is already
root on the target architecture.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | Yes | Name of the target to build, as defined in `starforge.yaml`. |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--executor` | string | `vm` | Build in the managed VM; `native` and compatibility mode `auto` are explicit alternatives. |
| `--clean` | bool | `false` | Force a full rebuild, ignoring the overlay cache. |

## Build Phases

Each phase is cached independently. If a phase's inputs change, it and all subsequent phases are rebuilt (cascade invalidation).

| # | Phase | Description |
|---|-------|-------------|
| 0 | preinstall | Write vconsole.conf for keymap (before pacstrap) |
| 1 | packages | Install packages via `pacstrap` |
| 2 | sysconfig | Set hostname, locale, timezone, keymap |
| 3 | users | Create groups and user accounts with groups and passwords |
| 4 | files | Create directories, copy/create/edit files, systemd unit files, copy/move/link/delete |
| 5 | permissions | Set file ownership (chown) and permissions (chmod) |
| 6 | services | Enable/disable/mask systemd units, set default target |
| 7 | boot | Configure systemd-boot loader and entries |
| 8 | scripts | Execute build scripts inside chroot |

## Cache Layout

```
.starforge/<target>/cache/
├── manifest.json           # Records hash and completion state per phase
├── empty/                  # Empty lowerdir for phase 0
├── 0-preinstall/upper/     # Phase 0 filesystem delta
├── 1-packages/upper/       # Phase 1 filesystem delta
├── ...
└── 8-scripts/upper/        # Phase 8 filesystem delta
```

## Examples

```bash
# Build a target
starforge build device

# Force a full rebuild
starforge build device --clean
```

## Requirements

- The VM executor requires `qemu-img`, `mkfs.ext4`, GnuPG, and the target QEMU system binary (`qemu-system-x86_64` or `qemu-system-aarch64`).
- KVM is optional. Same-architecture builds use it when `/dev/kvm` is accessible; cross-architecture builds use QEMU TCG emulation.
- Builder appliances are downloaded, signature-verified, generated, and cached automatically on first use.
- Source checkouts build the architecture-specific worker from the current source. Binary installations must include the matching `starforge-worker-linux-amd64` and `starforge-worker-linux-arm64` companion workers.

## See Also

- [write](write/) -- Write a target to a block device
- [export](export/) -- Export as a disk image or partition images
- [run](run/) -- Boot the build in QEMU for testing
- [clean](clean/) -- Remove build artifacts
