---
title: "Guide"
weight: 3
---

This guide covers every aspect of building OS images with StarForge, organized by topic. Start with project setup, then work through the configuration topics relevant to your image. Each page is self-contained and links to related reference material.

### Project Setup

- [Project Structure](project-structure/) -- The `starforge.yaml` file, targets, layer directories, and build artifacts.
- [Writing Layers](writing-layers/) -- Layer anatomy, step fields, `!include` splitting, override semantics, and best practices.

### Configuration

- [Partitions](partitions/) -- Defining disk layout with `partition-add`, growable sizes, and cross-layer modifications.
- [Packages](packages/) -- Installing and removing packages with `pacman-add` and `pacman-remove`.
- [System Configuration](system-configuration/) -- Hostname, locale, timezone, and keymap.
- [Users & Groups](users-and-groups/) -- Creating user accounts and groups with cross-layer merge support.

### Files & Services

- [Files & Directories](files-and-directories/) -- Creating, copying, editing, moving, deleting, and linking files.
- [Systemd Units](systemd-units/) -- Services, mounts, timers, sockets, slices, drop-in overrides, and user units.
- [Bootloader](bootloader/) -- Configuring systemd-boot with loader settings and boot entries.

### Advanced

- [Scripts](scripts/) -- Running shell scripts inside the chroot during build.
- [Variables](variables/) -- Variable substitution, imports, exports, and target args.
- [Remote Layers](remote-layers/) -- Git repositories, archives, and HTTP layer sources.
- [Multi-Target Projects](multi-target-projects/) -- Producing multiple OS variants from shared layers.
- [Installer](installer/) -- Building installer images for device provisioning.
- [Corona](corona-files/) -- Chunked image writes, `.corona` artifacts, and why they are useful for installers, recovery, and OTA systems.

### Build & Deploy

- [Building & Testing](building-and-testing/) -- Building targets, incremental caching, QEMU testing, and inspecting builds.
- [Deploying](deploying/) -- Writing to devices, exporting disk images, and exporting partition images.
