---
title: "Install Lifecycle Hooks"
weight: 60
---


At well-defined points during an install, the installer server runs every executable script under `/usr/lib/starforge/hooks/<phase>.d/` (in the running installer's filesystem) in lexical order. This lets a project layer arbitrary install-time logic onto the install pipeline without changing StarForge.

Drop scripts into the right `<phase>.d/` directory via `file-create` in any layer that's part of your installer target. Use a numeric prefix (`10-foo.sh`, `20-bar.sh`) to control order across multiple contributors.

## Lifecycle phases

| Phase | When it runs | Typical use |
|---|---|---|
| `pre-partition` | After payload + manifest load, before `sfdisk` writes the GPT | Confirm wipe, abort opportunities, custom partition pre-checks |
| `post-write` | After every partition image is dd'd onto its partition (and grow filesystems are expanded) | Image verification, raw-byte tweaks |
| `post-install` | After fstab regeneration, bootloader install, and `mkinitcpio -P` finish; **target rootfs is still mounted** | Stage extra files into specific partitions, register first-boot one-shots, capture diagnostics |
| `on-failure` | When any step (including a hook) fails | Cleanup, telemetry, surfacing diagnostics |

Hooks for the first three phases run sequentially; a non-zero exit aborts the install with that hook's error. `on-failure` hooks are best-effort — their errors are logged but don't compound the install error already in flight.

## Arguments

Each hook is invoked as:

```
<script> <target_rootfs> <payload_dir>
```

| Arg | What it points at |
|---|---|
| `target_rootfs` | The temporary mount point where every target partition is mounted at its declared `mount_point`. Empty string for `pre-partition` and `post-write` (rootfs not yet mounted) and best-effort for `on-failure`. |
| `payload_dir` | The directory on the installer USB that holds this install's `manifest.json` and `*.img.zst` files (i.e. the resolved path under `/images/<payload-name>/`). |

Hooks should treat both arguments as required and validate them, especially `target_rootfs` for the `post-install` phase.

## Output and logging

Hook stdout and stderr stream into the installation log line-by-line, alongside `genfstab` / `mkinitcpio` output. The TUI renders them live; the persisted log keeps the full transcript.

## Example: stage restore images on recovery partitions (Edge-OS)

```yaml
# layers/installer-base/layer.yaml
- action: file-create
  label: Stage restore images on recovery + fallback-recovery
  path: /usr/lib/starforge/hooks/post-install.d/10-stage-restore-images.sh
  mode: "0755"
  content: |
    #!/bin/sh
    set -e
    target_rootfs=$1
    payload_dir=$2

    stage_one() {
        label=$1; shift
        abs=$target_rootfs/$label
        [ -d "$abs" ] || { echo "$abs not present, skipping $label"; return 0; }
        restore=$abs/var/lib/telemetryos/restore
        mkdir -p "$restore"
        for img in "$@"; do
            cp -- "$payload_dir/$img.img.zst" "$restore/"
        done
        echo "Staged $* on $label"
    }

    stage_one recovery xbootldr root
    stage_one fallback-recovery xbootldr root recovery
```

The script reaches into `<target_rootfs>/recovery/` and `<target_rootfs>/fallback-recovery/` (which are mount points of the freshly-installed disk's recovery partitions) and copies compressed boot/root images into them so an operator can later flash them back if the active rootfs gets corrupted.

## Notes

- Scripts must be marked executable (`mode: "0755"` on the `file-create`). Non-executable files are silently skipped.
- Subdirectories under a `<phase>.d/` are ignored.
- Phase directories that don't exist are no-ops; you only create the ones you need.
- Hooks run with whatever privileges the installer server has — typically root, since the install pipeline needs to mount and write to disk.
