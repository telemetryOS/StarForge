# starforge build

Build disk images for a target.

## Synopsis

```
starforge build <target> [--clean]
```

## Description

Resolve layers for a target, execute build phases, and produce partition images. Creates sparse image files in the `.starforge/` build directory.

The build process follows three stages:

1. **Collect** -- Process all layers and validate the configuration.
2. **Execute** -- Run 9 build phases via overlayfs with per-phase caching.
3. **Package** -- Copy the merged filesystem into partition images.

Phases are cached using overlayfs -- unchanged phases are skipped on subsequent builds. Use `--clean` to force a full rebuild.

The command automatically elevates to root (via sudo) for the execute phase, then restores ownership of build artifacts to the invoking user.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | Yes | Name of the target to build, as defined in `starforge.yaml`. |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
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

- Root privileges (elevated automatically via sudo)
- All build tools (pacstrap, pacman, mkfs, sgdisk, etc.) are vendored automatically on first run

## See Also

- [write](write.md) -- Write a completed build to a storage device
- [export](export.md) -- Export as a disk image or partition images
- [run](run.md) -- Boot the build in QEMU for testing
- [clean](clean.md) -- Remove build artifacts
