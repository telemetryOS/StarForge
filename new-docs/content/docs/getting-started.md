---
title: "Getting Started"
weight: 2
---


This guide walks you through building your first custom Arch Linux image with StarForge, from installation to a running virtual machine.

## Prerequisites

StarForge runs on **any Linux distribution**. Build tools (pacstrap, pacman, mkfs, sgdisk) are vendored automatically on first use.

- **Linux** with overlayfs support (standard on all modern kernels)
- **Root access** for build operations
- **Go 1.21+** to build StarForge from source
- **Internet access** on first run to download vendored dependencies
- **QEMU** for `starforge run` -- the only host-installed dependency

```bash
sudo pacman -S qemu-full            # Arch Linux
sudo apt install qemu-system-x86    # Ubuntu/Debian
sudo dnf install qemu-system-x86    # Fedora
```

## Building StarForge

```bash
cd StarForgeNext
go build -o starforge ./cmd/starforge
sudo cp starforge /usr/local/bin/
```

## Creating a Project

```bash
starforge init my-os
cd my-os
```

This creates a `starforge.yaml` project file, a `.gitignore`, and a `layers/base/layer.yaml` starter layer.

The `starforge.yaml` defines your build targets -- each target references an ordered list of layers. See [Projects](concepts/projects/) for details.

```yaml
name: my-os
targets:
  distribution:
    layers:
      - ./layers/base
      - ./layers/desktop
```

Each layer is a directory containing a `layer.yaml` file, plus any files that actions reference (configs, scripts, etc.). See [Layers](concepts/layers/) for the full reference.

## Minimal Layer Example

Here is a base layer that produces a bootable system with partitions, packages, system configuration, a bootloader, a user account, and basic services:

```yaml
steps:
  - action: partition-add
    partitions:
      - name: boot
        filesystem: vfat
        size: 1G
        mount_point: /boot
        type: efi
      - name: root
        filesystem: ext4
        size: 12G
        mount_point: /
        type: linux

  - action: pacman-add
    packages:
      - base
      - linux
      - linux-firmware
      - sudo
      - networkmanager
      - openssh

  - action: system-hostname
    hostname: my-device

  - action: system-locale
    locale: en_US.UTF-8

  - action: system-timezone
    timezone: UTC

  - action: system-keymap
    keymap: us

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

  - action: system-user
    name: admin
    groups: [wheel]
    shell: /bin/bash
    password: changeme

  - action: systemd-service
    name: NetworkManager
    enable: true

  - action: systemd-service
    name: sshd
    enable: true
```

## Building

```bash
starforge build distribution
```

The first build downloads vendored tools, installs all packages, and runs all build phases. Subsequent builds use the overlay cache -- only phases whose inputs changed are re-executed. Use `--clean` to force a full rebuild.

## Inspecting the Build

Verify how your layers resolve before or after building:

```bash
starforge inspect distribution                      # Show everything
starforge inspect distribution packages             # Show the package list
starforge inspect distribution partitions --layers  # Partition layout with provenance
```

The `--layers` / `-l` flag shows which layer contributed each item. Available concerns: `partitions`, `packages`, `groups`, `users`, `services`, `files`, `permissions`, `boot`, `system`, `scripts`.

## Testing in QEMU

Boot your build in a virtual machine:

```bash
starforge run distribution
```

SSH into the VM from another terminal with `ssh -p 2222 localhost`. Use `--serial` for a serial console, or `--overlay <name>` to persist VM changes across reboots:

```bash
starforge run distribution --serial
starforge run distribution --overlay testing
```

## Deploying

Write directly to a USB drive or SD card:

```bash
starforge write distribution /dev/sdb
```

Export a single bootable disk image:

```bash
starforge export distribution disk --size 16G --output ./release/my-os.img
```

Export individual partition images for OTA or custom deployment:

```bash
starforge export distribution partitions --output ./release/
```

## Cleaning Up

```bash
starforge clean distribution          # Remove all build artifacts for a target
starforge clean distribution cache    # Remove only the cache
starforge clean deps                  # Remove vendored dependencies
```

## Next Steps

- [Guide](guide/) -- In-depth walkthrough of layer writing, advanced features, and multi-target projects
- [Concepts](concepts/) -- Architecture, build pipeline, caching, and override semantics
- [Actions Reference](actions/) -- Complete reference for all actions
- [Commands](commands/) -- CLI command reference
