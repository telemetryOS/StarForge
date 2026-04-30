---
title: "systemd-boot-install"
weight: 36
---


Configure the systemd-boot bootloader for the target system.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `loader` | object | No | Loader configuration (written to `loader/loader.conf` on the active boot partition). |
| `entries` | list of entry objects | No | Boot entries. Each entry's `.conf` file is written to `loader/entries/` on the partition the entry targets (see `extended` below). |

### Loader fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `default` | string | No | Default entry name (e.g., `arch.conf`). |
| `timeout` | int | No | Menu timeout in seconds. |
| `editor` | bool | No | Allow kernel command line editing. |

### Entry fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Entry filename (`.conf` is appended automatically if omitted). |
| `title` | string | Yes | Display title shown in the boot menu. |
| `kernel` | string | Yes | Kernel name as produced by `mkinitcpio` (e.g. `linux`, `linux-lts`). The engine derives the actual filenames as `vmlinuz-<kernel>` and `initramfs-<kernel>.img`. |
| `options` | string | Yes | Kernel command line options (e.g. `root=LABEL=root rw quiet`). The engine injects `root=UUID=<rootfs-uuid>` automatically if no `root=` is present. |
| `extended` | bool | No | Selects which boot partition the entry lives on. When unset, defaults to `true` if a partition with `type: xbootldr` is declared (entries on XBOOTLDR matches `bootctl`'s native behavior of putting actively-managed entries there), else `false`. Set explicitly to `true` to force XBOOTLDR (errors at build time if no such partition exists), or `false` to force the ESP — used when keeping a fallback recovery entry on a frozen ESP while XBOOTLDR holds the actively-updated entries. |
| `path` | string | No | Optional rootfs directory where the kernel/initrd files should live. Must be a subpath of the entry's partition mount point. Defaults to that mount point itself (so `/efi/vmlinuz-<kernel>` for an ESP entry, `/boot/vmlinuz-<kernel>` for an XBOOTLDR entry). |

## Kernel and initrd placement

The engine writes the entry `.conf` referencing `<path>/vmlinuz-<kernel>` and `<path>/initramfs-<kernel>.img`, and verifies those files exist at that location during phase 7. It does **not** copy them from elsewhere. The OS layer is responsible for placing the kernel and initramfs at the entry's resolved path.

Two common patterns:

1. **Mount the partition at `/boot`** (the canonical pacman destination). Pacstrap and mkinitcpio write `vmlinuz-<kernel>` / `initramfs-<kernel>.img` there directly, and an entry on that partition resolves them with no extra steps. This is the simplest setup.
2. **Use a `file-copy` action** before phase 7 to deposit the files at the entry's destination if it differs from where pacman wrote them.

If the kernel or initrd is missing at the entry's destination, the build errors with a clear message naming the file and the path that was checked.

## Example

```yaml
- action: systemd-boot-install
  loader:
    default: arch.conf
    timeout: 3
    editor: false
  entries:
    - name: arch.conf
      title: Arch Linux
      kernel: linux
      options: rw quiet                # root=UUID=... auto-injected
```

### Multiple entries with ESP/XBOOTLDR split

```yaml
- action: systemd-boot-install
  loader:
    default: arch+3-0.conf
    timeout: 0
    editor: false
  entries:
    - name: arch+3-0.conf
      title: TelemetryOS Edge
      kernel: linux                    # defaults to extended:true (XBOOTLDR present)
      options: rw quiet rootflags=noatime,commit=600 audit=0 noresume

    - name: recovery+3-0.conf
      title: TelemetryOS Recovery
      kernel: linux
      options: rw quiet rootflags=noatime,commit=600 noresume

    - name: fallback-recovery.conf
      title: TelemetryOS Fallback Recovery
      kernel: linux
      extended: false                  # force onto the frozen ESP
      options: rw quiet rootflags=noatime,commit=600 noresume
```

In this example, `arch` and `recovery` entries (and their kernels) live on the XBOOTLDR partition where pacman keeps `/boot/vmlinuz-linux`. The `fallback-recovery` entry is forced onto the ESP. To make this work, the fallback target should mount the ESP at `/boot` so its pacstrap puts the kernel/initramfs directly on the frozen ESP — the engine itself does not copy kernel files between partitions.

## Semantics

**Mixed.** The `loader` configuration is replaced by the last layer that defines it. Boot `entries` accumulate across layers — entries from later layers are appended to entries from earlier layers.

## Build Phase

Phase 7 (`boot`). The bootloader binary is installed via `bootctl install` after services are enabled, and the loader/entry files are written to the appropriate partitions based on each entry's `extended` flag.

## Notes

- The action requires at least one partition with `type: efi` (the ESP — `bootctl install` always writes `systemd-bootx64.efi` and `loader.conf` there).
- An optional partition with `type: xbootldr` enables the ESP/XBOOTLDR split documented in the Boot Loader Specification.
- The kernel package (typically `linux`) must be installed via `pacman-add`. Mkinitcpio's pacman hook deposits `vmlinuz-<kernel>` and `initramfs-<kernel>.img` at `/boot/` in the target rootfs. For the engine to find them when writing the entry, the partition that holds the entry must be mounted at `/boot`, or a `file-copy` action must move the files to the entry's partition mount point before phase 7.
