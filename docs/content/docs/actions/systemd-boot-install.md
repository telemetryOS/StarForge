---
title: "systemd-boot-install"
weight: 36
---


Configure the systemd-boot bootloader for the target system.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `loader` | object | No | Loader configuration (written to `loader/loader.conf` on the EFI partition). |
| `entries` | list of entry objects | No | Boot entries (written to `loader/entries/` on the EFI partition). |

### Loader fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `default` | string | No | Default entry name (e.g., `arch.conf`). |
| `timeout` | int | No | Menu timeout in seconds. |
| `editor` | bool | No | Allow kernel command line editing. |

### Entry fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Entry filename including extension (e.g., `arch.conf`). |
| `title` | string | Yes | Display title in the boot menu. |
| `linux` | string | Yes | Path to the kernel image (e.g., `/vmlinuz-linux`). |
| `initrd` | string | Yes | Path to the initramfs (e.g., `/initramfs-linux.img`). |
| `options` | string | Yes | Kernel command line options (e.g., `root=LABEL=root rw`). |
| `partition` | string | No | Where the `.conf` is written. Default (or `boot`) writes to `/boot/loader/entries/` — the XBOOTLDR partition if one exists, otherwise the ESP. `esp` forces the entry onto the ESP at `/efi/loader/entries/` — useful when XBOOTLDR holds the actively-managed entries and a specific entry must live on the frozen ESP for isolation (e.g. a fallback recovery entry that the updater must not be able to corrupt). Kernel and initrd paths inside the entry are resolved relative to the partition the `.conf` lives on, per the Boot Loader Specification. |

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
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: root=LABEL=root rw quiet
```

### Multiple entries

```yaml
- action: systemd-boot-install
  loader:
    default: arch.conf
    timeout: 5
    editor: true
  entries:
    - name: arch.conf
      title: Arch Linux
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: root=LABEL=root rw quiet
    - name: arch-fallback.conf
      title: Arch Linux (fallback)
      linux: /vmlinuz-linux
      initrd: /initramfs-linux-fallback.img
      options: root=LABEL=root rw
```

## Semantics

**Mixed.** The `loader` configuration is replaced by the last layer that defines it. Boot `entries` accumulate across layers -- entries from later layers are appended to entries from earlier layers.

## Build Phase

Phase 7 (`boot`). The bootloader is installed and configured after services are enabled.

## Notes

- This action requires an EFI partition (typically `type: efi` in `partition-add`).
- The action installs `systemd-boot` to the EFI partition and writes loader and entry configuration files.
- Kernel and initramfs files must exist in the target (typically provided by the `linux` package via `pacman-add`).
