---
title: "Bootloader"
weight: 9
---

StarForge uses systemd-boot as its bootloader. The `systemd-boot-install` action configures the loader settings and defines boot entries. This action requires an EFI partition in your partition layout (typically defined with `type: efi` in a [`partition-add`](../../actions/partition-add/) step).

## Basic Configuration

A `systemd-boot-install` step has two sections: `loader` (global bootloader settings) and `entries` (the list of boot menu items).

```yaml
- action: systemd-boot-install
  loader:
    default: arch.conf
    timeout: 0
    editor: false
  entries:
    - name: arch
      title: My OS
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet splash
```

This produces two files on the EFI partition:

**`loader/loader.conf`**:
```ini
default arch.conf
timeout 0
editor no
```

**`loader/entries/arch.conf`**:
```
title   My OS
linux   /vmlinuz-linux
initrd  /initramfs-linux.img
options root=UUID=... rw quiet splash
```

## Loader Settings

The `loader` section controls how systemd-boot behaves at startup.

| Field | Type | Description |
|-------|------|-------------|
| `default` | string | Filename of the default boot entry (e.g., `arch.conf`). |
| `timeout` | int | Seconds to display the boot menu. Set to `0` to boot immediately without showing the menu. |
| `editor` | bool | Whether to allow editing the kernel command line at boot time. Set to `false` for production images. |

## Boot Entries

Each entry in the `entries` list defines a boot menu item.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Entry filename (e.g., `arch`). The `.conf` extension is added automatically. |
| `title` | string | Yes | Display title shown in the boot menu. |
| `linux` | string | Yes | Path to the kernel image (e.g., `/vmlinuz-linux`). |
| `initrd` | string | Yes | Path to the initramfs image (e.g., `/initramfs-linux.img`). |
| `options` | string | Yes | Kernel command line options (e.g., `rw quiet splash`). |

### Automatic Root UUID Injection

StarForge automatically prepends `root=UUID=...` to the `options` field based on the partition mounted at `/` in your partition layout. You do not need to specify the root device manually. This means your boot entries remain portable across different disks and devices.

## Multiple Boot Entries

You can define multiple entries for different boot modes. The `default` field in `loader` determines which entry boots automatically.

```yaml
- action: systemd-boot-install
  loader:
    default: arch.conf
    timeout: 5
    editor: true
  entries:
    - name: arch
      title: My OS
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet splash

    - name: arch-recovery
      title: My OS (Recovery)
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw single

    - name: arch-fallback
      title: My OS (Fallback Initramfs)
      linux: /vmlinuz-linux
      initrd: /initramfs-linux-fallback.img
      options: rw
```

This creates three entries in `loader/entries/`. The main entry boots normally, the recovery entry drops to single-user mode, and the fallback entry uses the fallback initramfs for hardware compatibility troubleshooting.

## Replace Semantics

The `systemd-boot-install` action uses **replace** semantics. If multiple layers define this action, the last layer's configuration replaces any earlier one entirely. There is no merging of loader settings or entry lists across layers.

This means a later layer can completely redefine the bootloader configuration:

```yaml
# Base layer defines a simple boot entry
- action: systemd-boot-install
  loader:
    default: arch.conf
    timeout: 0
    editor: false
  entries:
    - name: arch
      title: Base OS
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet

# A later layer replaces the entire configuration
- action: systemd-boot-install
  loader:
    default: kiosk.conf
    timeout: 0
    editor: false
  entries:
    - name: kiosk
      title: Kiosk OS
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet splash loglevel=0
```

In this example, the final image will have only the `kiosk.conf` entry. The base layer's `arch.conf` entry is discarded entirely.

## Prerequisites

The `systemd-boot-install` action requires:

- An **EFI partition** defined via `partition-add` with `type: efi`. This is where the bootloader and its configuration files are installed.
- The **kernel and initramfs** files to exist in the target filesystem. These are typically provided by the `linux` package installed via `pacman-add`.

## Build Phase

Bootloader configuration runs in phase 7 (`boot`), after services are enabled (phase 6) and before scripts (phase 8). The action installs systemd-boot to the EFI partition and writes the loader and entry configuration files.

## See Also

- [systemd-boot-install reference](../../actions/systemd-boot-install/) -- Complete field reference.
- [Partitions](../partitions/) -- Defining the EFI partition with `partition-add`.
- [Packages](../packages/) -- Installing the `linux` package that provides kernel and initramfs files.
