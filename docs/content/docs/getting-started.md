---
title: "Getting Started"
weight: 2
---


This tutorial walks you from zero to a running virtual machine with StarForge. By the end, you will have built a bootable Arch Linux image and tested it in QEMU.

## Prerequisites

StarForge runs on **any Linux distribution**. It builds inside a managed, architecture-specific QEMU appliance by default, so you do not need an Arch Linux host or root access.

- **Linux**
- **Internet access** on first run to download and verify the builder appliance
- **QEMU**, `qemu-img`, `mkfs.ext4`, and GnuPG
- **UEFI firmware** for each target architecture you want to boot with `starforge run`

```bash
sudo pacman -S qemu-full edk2-ovmf edk2-aarch64  # Arch Linux
sudo apt install qemu-system-x86 qemu-system-arm qemu-utils ovmf qemu-efi-aarch64 e2fsprogs gnupg  # Ubuntu/Debian
```

KVM is optional. StarForge uses KVM for a same-architecture target when the user can open `/dev/kvm`; otherwise it uses QEMU TCG. Cross-architecture builds always use TCG.

## Install StarForge

Download the latest release and install:

```bash
curl -Lo starforge https://github.com/telemetryos/starforge/releases/latest/download/starforge-linux-amd64
chmod +x starforge
sudo mv starforge /usr/local/bin/
```

## Create a Project

Use `starforge init` to scaffold a new project:

```bash
starforge init my-os
cd my-os
```

You will be prompted for an optional description and a target name (press Enter to accept the default `distribution`). This creates:

```
my-os/
├── starforge.yaml          # Project definition with one target
├── .gitignore              # Excludes .starforge/ build directory
└── layers/
    └── base/
        ├── layer.yaml      # Starter base layer
        └── files/
            └── etc/        # Layer files directory
```

The generated `starforge.yaml` defines a single target that references the base layer:

```yaml
name: "my-os"
targets:
  distribution:
    platform:
      architecture: x86_64
      machine: generic-uefi
      distribution: archlinux
    layers:
      - ./layers/base
```

## Customize the Base Layer

Replace the generated `layers/base/layer.yaml` with a more complete base layer. This example produces a bootable system with networking, SSH, a user account, and a bootloader:

```yaml
steps:
  # Disk layout: EFI boot partition + root filesystem
  - action: partition-add
    partitions:
      - name: boot
        filesystem: vfat
        size: 512M
        mount_point: /boot
        type: efi
      - name: root
        filesystem: ext4
        size: 8G
        mount_point: /

  # Packages installed via pacstrap
  - action: pacman-add
    packages:
      - base
      - linux
      - linux-firmware
      - sudo
      - networkmanager
      - openssh

  # System identity
  - action: system-hostname
    hostname: my-os

  - action: system-locale
    locale: en_US.UTF-8
    locales:
      - en_US.UTF-8 UTF-8

  - action: system-timezone
    timezone: UTC

  - action: system-keymap
    keymap: us

  # Bootloader: systemd-boot with a single entry.
  # The root=UUID=... kernel parameter is injected automatically.
  - action: systemd-boot-install
    loader:
      default: arch.conf
      timeout: 0
      editor: false
    entries:
      - name: arch.conf
        title: My OS
        linux: /vmlinuz-linux
        initrd: /initramfs-linux.img
        options: rw quiet

  # User account with sudo access via the wheel group
  - action: system-user
    name: admin
    groups: [wheel]
    shell: /bin/bash
    password: changeme

  # Enable networking and SSH at boot
  - action: systemd-service
    name: NetworkManager
    enable: true

  - action: systemd-service
    name: sshd
    enable: true
```

Each step maps to a StarForge action. The `partition-add` step defines the disk layout. The `pacman-add` step lists packages to install via pacstrap. The four system steps (`system-hostname`, `system-locale`, `system-timezone`, `system-keymap`) configure the OS identity. The `systemd-boot-install` step sets up the bootloader --- the root partition UUID is injected automatically, so you do not need to specify it. The `system-user` step creates a user account, and the two `systemd-service` steps enable services at boot.

For the full list of actions and their fields, see the [Actions Reference](actions/).

## Build

Build the `distribution` target:

```bash
starforge build distribution
```

The first build downloads and signature-verifies the target-architecture bootstrap, generates and caches the builder appliance, and runs all 9 build phases. This takes several minutes depending on emulation, network speed, and the package list. Subsequent builds reuse the appliance, package cache, and phase cache.

The VM executor is always the default, including when StarForge is launched by
root. This keeps host privileges and installed build tools out of the normal
workflow. Select native execution explicitly when it is genuinely needed:

```bash
starforge --executor=native build distribution
starforge --executor=auto build distribution
```

Native x86-64 execution uses StarForge's vendored tool bundle. Native ARM64
execution validates and uses the Arch build tools installed on the host.
`--executor=auto` is retained for compatibility and may choose native execution
only when StarForge is already root on the target architecture.

Use `--clean` to force a full rebuild:

```bash
starforge build distribution --clean
```

## Inspect

Before or after building, use `starforge inspect` to verify how your layers resolve without running a full build:

```bash
# Show everything: partitions, packages, system config, users, services, boot, etc.
starforge inspect distribution

# Show just the resolved package list
starforge inspect distribution packages

# Show partition layout with which layer defined each partition
starforge inspect distribution partitions --layers
```

The `--layers` (`-l`) flag shows which layer contributed each item, making it easy to trace override behavior in multi-layer projects. Available concerns: `partitions`, `packages`, `groups`, `users`, `services`, `files`, `permissions`, `boot`, `system`, `scripts`, `installer`.

## Test in QEMU

Boot your build in a virtual machine:

```bash
starforge run distribution
```

This launches QEMU with your built image, forwarding port 2222 on the host to port 22 in the VM. From another terminal, SSH into the running VM:

```bash
ssh -p 2222 admin@localhost
```

Use `--serial` to attach a serial console for kernel boot messages:

```bash
starforge run distribution --serial
```

The target VM is a host-side sibling of the builder VM. It boots the sealed full-disk artifact through a disposable QEMU snapshot, so testing cannot mutate the artifact and all runtime changes are discarded when QEMU exits.

## Add an ARM64 Target

The executor and target platform are independent. An x86-64 host can build an ARM64 target by declaring it in `starforge.yaml`:

```yaml
targets:
  distribution-arm64:
    platform:
      architecture: aarch64
      machine: generic-uefi
      distribution: archlinuxarm
    layers:
      - ./layers/base-arm64
```

ARM layers must select ARM packages and use the filenames installed by their kernel package. For example, Arch Linux ARM's `linux-aarch64` package uses `Image` and `initramfs-linux.img`:

```yaml
- action: systemd-boot-install
  entries:
    - name: arch-arm64.conf
      title: My ARM64 OS
      kernel: linux-aarch64
      kernel_file: Image
      initrd_file: initramfs-linux.img
      options: rw console=ttyAMA0,115200
```

Arch Linux Ports is also available by setting `distribution: archlinux-ports`. Its AArch64 repository follows the standard Arch kernel layout, so use `kernel: linux` with the default `vmlinuz-linux` and `initramfs-linux.img` filenames. Repository signing is verified with the Arch Linux keyring and the pinned Ports repository key.

Build it from any supported Linux host with:

```bash
starforge --executor=vm build distribution-arm64
starforge run --serial distribution-arm64
```

## Deploy

Write directly to a USB drive or SD card:

```bash
starforge write distribution /dev/sdX
```

You will be prompted to confirm before any data is written.

Export a single bootable disk image for flashing or distribution:

```bash
starforge export distribution disk ./release/my-os.img
```

Export individual partition images for OTA update systems or custom deployment workflows:

```bash
starforge export distribution partitions ./release/
```

## Next Steps

This tutorial covered a single-layer, single-target project. StarForge supports much more:

- **Adding layers** --- Split your configuration across base, feature, and variant layers. See the [Complete Guide](guide/) for layering strategy and multi-target projects.
- **Systemd services** --- Create unit files, drop-in overrides, mounts, timers, sockets, and slices. See [Systemd Units](guide/systemd-units/).
- **Variables** --- Parameterize layers with `${{ variables }}` for reuse across targets. See [Variables](guide/variables/).
- **Multi-target projects** --- Build different OS variants (minimal, desktop, kiosk) from shared layers. See [Multi-Target Projects](guide/multi-target-projects/).
- **Installer system** --- Build self-installing images with a REST API server and client UI. See the [Installer System](guide/installer/).
- **Remote layers** --- Pull shared layers from git repositories, archives, or HTTP URLs. See [Remote Layers](guide/remote-layers/).
