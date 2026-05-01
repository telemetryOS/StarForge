---
title: "Installer System"
weight: 14
---

StarForge can build self-installing images that boot from USB and write an OS to a target device's internal storage. The installer system has three components: payloads, a server, and a client.

## Architecture

The installer is itself a bootable Linux system built as a StarForge target. At boot, the `starforge-install-server` daemon starts as a systemd service and provides a REST API on a configurable port (default 8100). The `starforge-install` TUI client auto-starts on the configured TTY via a getty autologin override and communicates with the server over HTTP.

The three components:

1. **Payloads** -- Built OS targets bundled as compressed partition images into the installer image. Each payload includes a `manifest.json` describing its partitions.
2. **Server** -- A REST API daemon (`starforge-install-server`) that manages disk detection, partition writing, and installation progress.
3. **Client** -- A terminal UI (`starforge-install`) that guides the user through payload selection, disk selection, and installation.

The TUI walks the user through:

1. Selecting a payload (skipped automatically if only one is available)
2. Selecting a target disk from detected block devices
3. Confirming the installation (all data on the target disk will be destroyed)
4. Monitoring progress as partitions are written
5. Rebooting after completion

## Workflow

Building an installer is a two-step process -- first build the OS target(s) that will become payloads, then build the installer target that bundles them:

1. **Build the payload target(s)** -- these are the OS images to be installed:
   ```bash
   starforge build device
   ```

2. **Build the installer target** -- its layers use `install-payload` to bundle the device target, and `install-server`/`install-client` to configure the runtime:
   ```bash
   starforge build installer
   ```

3. **Write the installer to a USB drive** or export it as a disk image:
   ```bash
   starforge write installer /dev/sdX
   ```

4. **Boot the target machine** from the installer USB. The TUI guides disk selection and installation.

## Project Structure

A project with an installer defines at least two targets -- the OS to be installed and the installer itself:

```yaml
name: my-os
description: My custom OS with installer

targets:
  device:
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/app

  installer:
    layers:
      - ./layers/installer
```

The installer target is a minimal Linux system that includes the payload images and the installer binaries. It typically needs only a base set of packages, a bootloader, and the three installer actions.

For testing the installer in QEMU, add a `qemu` section with an extra disk to simulate the target device:

```yaml
  installer:
    qemu:
      disks:
        - name: install-target
          size: 32G
    layers:
      - ./layers/installer-base
      - ./layers/installer
```

## The Three Installer Actions

### install-payload

Bundles a built target's partition images as compressed payloads. The `target` field references another target by name -- that target must be built first.

```yaml
- action: install-payload
  target: device
  path: /images/device
```

StarForge reads the named target's partition images from the build directory, compresses them with zstd, and stores them at the specified `path` inside the installer's root filesystem. Both `target` and `path` are required. Each payload directory contains a `manifest.json` and compressed partition images (`*.img.zst`).

See the [`install-payload` reference](../../actions/install-payload/) for all fields.

### install-server

Configures the `starforge-install-server` daemon. Sets the listening port and payload directory. Also adds runtime dependencies (`dosfstools`, `e2fsprogs`, `arch-install-scripts`, `zstd`, `python`, `python-six`) to the package list automatically.

```yaml
- action: install-server
  port: 8100
```

Defaults to port 8100 and payload directory `/usr/lib/starforge/payloads` if not specified.

See the [`install-server` reference](../../actions/install-server/) for all fields.

### install-client

Configures the `starforge-install` TUI client. Sets which TTY auto-starts the installer interface.

```yaml
- action: install-client
  auto_login: tty1
```

The client only activates on the specified TTY -- SSH and serial sessions get a normal shell.

See the [`install-client` reference](../../actions/install-client/) for all fields.

## Example Installer Layer

A complete installer layer using all three actions:

```yaml
steps:
  - action: partition-add
    partitions:
      - name: boot
        filesystem: vfat
        size: 512M
        mount_point: /boot
        type: efi
      - name: root
        filesystem: ext4
        size: 4G
        mount_point: /
        type: linux

  - action: pacman-add
    label: Installer base packages
    packages:
      - base
      - linux
      - linux-firmware

  - action: system-hostname
    hostname: installer

  - action: systemd-boot-install
    loader:
      default: installer.conf
      timeout: 0
      editor: false
    entries:
      - name: installer.conf
        title: OS Installer
        linux: /vmlinuz-linux
        initrd: /initramfs-linux.img
        options: rw quiet

  - action: install-payload
    target: device
    path: /images/device

  - action: install-server
    port: 8100
    path: /images

  - action: install-client
    auto_login: tty1
```

## Building and Testing

Always build the payload target(s) before building the installer, since `install-payload` reads partition images from the payload target's build directory:

```bash
# Build the OS target first
starforge build device

# Then build the installer
starforge build installer
```

Test the installer in QEMU:

```bash
starforge run installer
```

The TUI will start on the configured TTY. In a QEMU environment, disk detection shows virtual disks. If the installer target's `qemu` section defines additional disks, those appear as installation targets.

To write the installer to a physical USB drive:

```bash
starforge write installer /dev/sdX
```

Or export as a disk image for distribution:

```bash
starforge export installer disk --size 8G --output ./release/installer.img
```

## Runtime Details

**Payload storage.** Payloads are stored at `/usr/lib/starforge/payloads/<target>/` inside the installer image. Each payload directory contains a `manifest.json` describing the partition layout and compressed partition images (`*.img.zst`).

**Server.** The `starforge-install-server` binary runs as a systemd service (`starforge-install-server.service`) and listens on the configured port. It exposes REST endpoints for listing payloads, detecting disks, starting installations, and polling progress.

**Client.** The `starforge-install` binary is launched via a getty autologin drop-in on the configured TTY. It runs as root and only activates on the specified TTY -- SSH and serial sessions get a normal shell.

**Installation pipeline.** When an installation starts, the server:

1. Partitions the target disk with GPT via sfdisk
2. Writes each compressed partition image directly to the corresponding device partition with `bmaptool`
3. Expands growable partitions to fill available space
4. Creates filesystems on expanded partitions
5. Regenerates fstab with the correct UUIDs from the target disk and installs the bootloader

## Companion Binaries

The installer requires two companion binaries built from the StarForge source tree:

- `starforge-install-server` (from `cmd/starforge-install-server/`)
- `starforge-install` (from `cmd/starforge-install/`)

These are built automatically during `starforge build` when installer actions are present. StarForge locates its own source tree and runs `go build` to produce the binaries, then bundles them into the installer image. No manual compilation is required.

## See Also

- [Multi-Target Projects](../multi-target-projects/) -- Structuring projects with device and installer targets.
- [Building & Testing](../building-and-testing/) -- Build commands and QEMU testing.
- [Deploying](../deploying/) -- Writing images to devices and exporting disk images.
- [`install-payload`](../../actions/install-payload/) -- Payload action reference.
- [`install-server`](../../actions/install-server/) -- Server action reference.
- [`install-client`](../../actions/install-client/) -- Client action reference.
