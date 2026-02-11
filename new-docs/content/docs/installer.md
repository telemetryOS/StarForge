---
title: "Installer System"
weight: 8
---


StarForge can build self-installing images. The installer system has three components:

1. **Payloads** -- Built OS targets bundled as compressed partition images into the installer image.
2. **Server** -- A REST API daemon (`starforge-install-server`) that manages disk detection, partition writing, and installation progress.
3. **Client** -- A terminal UI (`starforge-install`) that guides the user through payload selection, disk selection, and installation.

## Architecture

The installer runs as a bootable Linux system. At boot, `starforge-install-server` starts as a systemd service and provides a REST API on a configurable port (default 8100). The `starforge-install` TUI client auto-starts on the configured TTY via a getty autologin override and communicates with the server over HTTP.

The TUI walks the user through:

1. Selecting a payload (skipped automatically if only one is available)
2. Selecting a target disk from detected block devices
3. Confirming the installation (all data on the target disk will be destroyed)
4. Monitoring progress as partitions are written
5. Rebooting after completion

## Workflow

1. Build the payload target(s) first -- these are the OS images to be installed:
   ```bash
   starforge build device
   ```
2. Build the installer target -- its layers use `install-payload` to bundle the device target, and `install-server`/`install-client` to configure the runtime:
   ```bash
   starforge build installer
   ```
3. Write the installer to a USB drive or export it as a disk image:
   ```bash
   starforge write installer /dev/sdX
   ```
4. Boot the target machine from the installer USB. The TUI guides disk selection and installation.

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

## Actions

The installer subsystem uses three actions:

- [install-payload](actions/install-payload/) -- Bundle a built target's partition images as compressed payloads. Each payload is stored at the configured path with a `manifest.json` describing its partitions.
- [install-server](actions/install-server/) -- Configure the `starforge-install-server` daemon. Sets the listening port and payload directory. Also adds runtime dependencies (`dosfstools`, `e2fsprogs`, `zstd`) to the package list.
- [install-client](actions/install-client/) -- Configure the `starforge-install` TUI client. Sets the TTY for autologin so the installer starts automatically on boot.

## Example Layer

An installer layer using all three actions:

```yaml
steps:
  - action: install-payload
    target: device

  - action: install-server
    port: 8100

  - action: install-client
    auto_login: tty1
```

The `install-payload` action references a target by name. That target must be built before the installer target -- StarForge reads its partition images from the build directory and compresses them with zstd into the installer's root filesystem.

The `install-server` action defaults to port 8100 and payload directory `/usr/lib/starforge/payloads` if not specified. The `install-client` action configures which TTY auto-starts the TUI.

## Building and Testing

Build the payload target first, then the installer:

```bash
starforge build device
starforge build installer
```

Test the installer in QEMU:

```bash
starforge run installer
```

The TUI will start on the configured TTY. In a QEMU environment, disk detection will show virtual disks.

## Runtime Details

**Payload storage**: Payloads are stored at `/usr/lib/starforge/payloads/<target>/` inside the installer image. Each payload directory contains a `manifest.json` and compressed partition images (`*.img.zst`).

**Server**: The `starforge-install-server` binary runs as a systemd service (`starforge-install-server.service`) and listens on the configured port. It exposes REST endpoints for listing payloads, detecting disks, starting installations, and polling progress.

**Client**: The `starforge-install` binary is launched via a getty autologin drop-in on the configured TTY. It runs as root and only activates on the specified TTY -- SSH and serial sessions get a normal shell.

**Installation pipeline**: When an installation starts, the server partitions the target disk with GPT via sfdisk, writes each compressed partition image directly to the corresponding device partition (piping zstd decompression into dd), expands growable partitions to fill available space, and regenerates fstab and bootloader entries with the correct UUIDs from the target disk.

## Companion Binaries

The installer requires two companion binaries built from the StarForge source tree:

- `starforge-install-server` (from `cmd/starforge-install-server/`)
- `starforge-install` (from `cmd/starforge-install/`)

These are built automatically during `starforge build` when installer actions are present. StarForge locates its own source tree and runs `go build` to produce the binaries, then bundles them into the installer image.
