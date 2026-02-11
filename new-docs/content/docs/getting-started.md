---
title: "Getting Started"
weight: 2
---


This tutorial walks you from zero to a running virtual machine with StarForge. By the end, you will have built a bootable Arch Linux image and tested it in QEMU.

## Prerequisites

StarForge runs on **any Linux distribution**. Build tools like pacstrap, pacman, mkfs, and sgdisk are vendored automatically on first use --- you do not need an Arch Linux host.

- **Linux** with overlayfs support (standard on all modern kernels)
- **Root access** for build operations
- **Internet access** on first run to download vendored dependencies
- **QEMU** for testing --- the only host-installed dependency

```bash
sudo pacman -S qemu-full            # Arch Linux
sudo apt install qemu-system-x86    # Ubuntu/Debian
sudo dnf install qemu-system-x86    # Fedora
```

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
        └── layer.yaml      # Starter base layer
```

The generated `starforge.yaml` defines a single target that references the base layer:

```yaml
name: "my-os"
targets:
  distribution:
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

The first build downloads vendored tools (pacstrap, pacman, mkfs, and others) and runs all 9 build phases. This takes several minutes depending on your internet connection and the number of packages. Subsequent builds use the overlay cache --- only phases whose inputs changed are re-executed.

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

The `--layers` (`-l`) flag shows which layer contributed each item, making it easy to trace override behavior in multi-layer projects. Available concerns: `partitions`, `packages`, `groups`, `users`, `services`, `files`, `permissions`, `boot`, `system`, `scripts`.

## Test in QEMU

Boot your build in a virtual machine:

```bash
starforge run distribution
```

This launches QEMU with your built image, forwarding port 2222 on the host to port 22 in the VM. From another terminal, SSH into the running VM:

```bash
ssh -p 2222 admin@localhost
```

Use `--serial` to attach a serial console for kernel boot messages, or `--overlay` to persist VM changes across reboots:

```bash
starforge run distribution --serial
starforge run distribution --overlay testing
```

Named overlays are stored in `.starforge/distribution/overlays/` and can also be accessed with `starforge chroot distribution --overlay testing` for shell debugging.

## Deploy

Write directly to a USB drive or SD card:

```bash
starforge write distribution /dev/sdX
```

You will be prompted to confirm before any data is written.

Export a single bootable disk image for flashing or distribution:

```bash
starforge export distribution disk --size 16G --output ./release/my-os.img
```

Export individual partition images for OTA update systems or custom deployment workflows:

```bash
starforge export distribution partitions --output ./release/
```

## Next Steps

This tutorial covered a single-layer, single-target project. StarForge supports much more:

- **Adding layers** --- Split your configuration across base, feature, and variant layers. See the [Complete Guide](guide/) for layering strategy and multi-target projects.
- **Systemd services** --- Create unit files, drop-in overrides, mounts, timers, sockets, and slices. See [Systemd Units](guide/systemd-units/).
- **Variables** --- Parameterize layers with `${{ variables }}` for reuse across targets. See [Variables](guide/variables/).
- **Multi-target projects** --- Build different OS variants (minimal, desktop, kiosk) from shared layers. See [Multi-Target Projects](guide/multi-target-projects/).
- **Installer system** --- Build self-installing images with a REST API server and client UI. See the [Installer System](guide/installer/).
- **Remote layers** --- Pull shared layers from git repositories, archives, or HTTP URLs. See [Remote Layers](guide/remote-layers/).
