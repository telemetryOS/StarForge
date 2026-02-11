# Build Pipeline

StarForge builds in three stages: Collect, Execute, and Package. Each stage has a distinct purpose, and the Execute stage is further divided into 9 cached phases.

## Three Stages

### 1. Collect

Process all [layers](layers.md) in order, running each action to accumulate the desired state into a `BuildContext`. No filesystem changes happen during this stage -- actions are purely declarative. Variable substitution, `!include` resolution, and `layer-run` scripts all execute during Collect.

### 2. Execute

Run 9 sequential build phases. Each phase reads from the collected `BuildContext` and applies changes to the target rootfs using overlayfs layers. Phases are cached individually so unchanged phases are skipped on subsequent builds.

### 3. Package

Mount all overlayfs layers as a read-only merged view and copy the filesystem into partition images (e.g., `boot.img`, `root.img`). This stage always runs after Execute completes.

## Build Phases

| Phase | Name | Actions Executed |
|-------|------|-----------------|
| 0 | `preinstall` | `system-keymap` (writes `vconsole.conf` before pacstrap so `mkinitcpio` picks it up) |
| 1 | `packages` | `pacman-add` (runs `pacstrap` with deduplicated package list, initializes pacman keyring) |
| 2 | `sysconfig` | `system-hostname`, `system-locale`, `system-timezone`, `system-keymap` |
| 3 | `users` | `system-group`, `system-user` |
| 4 | `files` | `file-mkdir`, `file-create`, `file-edit`, `file-copy`, `file-move`, `file-link`, `file-delete`, all systemd unit file creation |
| 5 | `permissions` | `file-ownership`, `file-permissions` |
| 6 | `services` | `systemd-service`, `systemd-mount`, `systemd-timer`, `systemd-socket`, `systemd-slice`, `systemd-target` (enable/disable/mask/set-default) |
| 7 | `boot` | `systemd-boot-install` |
| 8 | `scripts` | `run` |

### Within Phase 4 (Files)

File operations execute in a fixed sub-order:

1. **Directories** -- `file-mkdir`
2. **Layer copies** -- `file-create` with directory `layer_path`, systemd unit files from inline definitions
3. **File creates** -- `file-create` with content or single-file `layer_path`
4. **File edits** -- `file-edit`
5. **Internal copies** -- `file-copy` (within target filesystem)
6. **Moves** -- `file-move`
7. **Links** -- `file-link`
8. **Deletes** -- `file-delete`

### Within Phase 5 (Permissions)

Ownership changes (`file-ownership` / chown) run before permission changes (`file-permissions` / chmod).

## OverlayFS Caching

Each phase produces an overlayfs upper directory that captures only the changes made during that phase. A SHA-256 hash of each phase's inputs is recorded in `manifest.json`. On subsequent builds, unchanged phases are skipped entirely.

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

The `manifest.json` file tracks each phase's hash, completion status, and the cache version:

```json
{
  "version": 1,
  "phases": {
    "0-preinstall": { "hash": "abc123...", "completed": true },
    "1-packages":   { "hash": "def456...", "completed": true }
  }
}
```

### Cascade Invalidation

If phase N changes (its input hash differs from the cached hash), phases N through 8 are all invalidated. This ensures correctness -- later phases may depend on the output of earlier ones.

For example, adding a new package invalidates phase 1 (packages) and all subsequent phases (sysconfig, users, files, permissions, services, boot, scripts).

### `--clean` Flag

Use `starforge build <target> --clean` to force a full rebuild by deleting the entire cache before starting. This is useful when the cache format changes or when debugging build issues.

Caches created by an older version of StarForge are automatically cleaned when a newer version detects a version mismatch.

## Build Directory Structure

All build artifacts are stored in `.starforge/` within the project root:

```
.starforge/<target>/
  cache/                       # Overlay cache (see above)
  overlays/                    # Named overlays (from --overlay flag)
    <name>/                    # Copies of partition images with persistent changes
  merged/                      # Overlay mount point (temporary, during build)
  boot.img                     # Partition images (output)
  root.img
  ...
```

This directory is added to `.gitignore` by `starforge init`.

## Named Overlays

The `--overlay <name>` flag on `starforge chroot` and `starforge run` creates a named overlay that persists changes across sessions. Named overlays are copies of the partition images stored in `.starforge/<target>/overlays/<name>/`.

When the underlying build changes (a new `starforge build` runs), named overlays are automatically invalidated and recreated from the fresh partition images on next use.

Overlay names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`.
