---
title: "Multi-Target Projects"
weight: 13
---

A StarForge project can define multiple targets that share layers to produce different OS variants from a common base. Each target builds independently with its own cache, but shared layers are defined once and reused across targets.

## Multiple Targets from Shared Layers

The simplest multi-target project shares a base layer and adds variant-specific layers on top:

```yaml
name: my-os
description: Multi-variant OS

targets:
  minimal:
    layers:
      - ./layers/base

  desktop:
    layers:
      - ./layers/base
      - ./layers/desktop

  kiosk:
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/kiosk
```

Build and test individual targets independently:

```bash
starforge build minimal
starforge build kiosk
starforge run kiosk --serial
```

## Layering Strategy

Effective multi-target projects organize layers by concern, with each layer serving a clear role:

**Base layer** -- Partitions, core packages (`base`, `linux`, `linux-firmware`), system identity (hostname, locale, timezone), core users, and essential services (networking, SSH, time sync). This layer is shared by every target.

**Feature layers** -- Additional capabilities like desktop environments, audio stacks, or networking tools. These build on the base without repeating its configuration.

**Variant layers** -- Application-specific configuration such as kiosk lockdown, autologin, or custom service definitions. These are the differentiating layers between targets.

**Development layers** -- Debug tools, SSH access, editor packages, and other utilities useful during development. Added only to development targets.

A typical directory structure:

```
layers/
├── base/              # Partitions, core packages, system config, users
├── desktop/           # GUI packages, compositor, audio
├── app/               # Application binary and service configuration
├── kiosk/             # Autologin, lockdown, kiosk-specific services
└── development/       # SSH, git, editors, debug packages
```

## Independent Caching

Each target maintains its own build cache under `.starforge/<target>/cache/`. Targets that share layers do not share cache directories.

```
.starforge/
├── minimal/
│   └── cache/
├── desktop/
│   └── cache/
└── kiosk/
    └── cache/
```

When a shared layer changes, all targets that include it are affected. For example, modifying the base layer invalidates the cached phases for `minimal`, `desktop`, and `kiosk` alike. Changing a layer that only the `kiosk` target uses affects only the `kiosk` cache.

Remote source caches (git repos, archives) under `.starforge/cache/sources/` are shared across targets, so a remote layer URL that appears in multiple targets is only fetched once.

## Best Practices

**One concern per layer.** Keep the base layer focused on core system configuration. Add capabilities through separate feature layers rather than combining unrelated concerns. This maximizes layer reuse across targets.

**Declare partitions early.** Define the full partition layout in the base layer. Later layers can modify individual partitions with [`partition-change`](../../actions/partition-change/) without redeclaring the entire layout:

```yaml
# base/layer.yaml -- defines the initial layout
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
        size: 8G
        mount_point: /
        type: linux

# kiosk/layer.yaml -- increases root partition for the kiosk variant
steps:
  - action: partition-change
    name: root
    size: 16G
```

**Use `!include` for large layers.** Split a layer's steps across multiple files by concern to keep each file manageable:

```yaml
steps:
  - !include ./packages.yaml
  - !include ./services.yaml
  - !include ./files.yaml
```

**Use labels on steps.** The `label` field provides readable output in [`starforge inspect`](../../commands/inspect/) and build logs:

```yaml
steps:
  - action: pacman-add
    label: Desktop compositor packages
    packages:
      - sway
      - swaybg
      - xdg-desktop-portal-wlr
```

**Prefer `file-create` with `layer_path` over inline content.** Keeping configuration files as actual files in your layer directory makes them easier to maintain, diff, and review:

```yaml
# Preferred -- file lives in the layer directory
- action: file-create
  layer_path: ./etc/sway/config
  path: /etc/sway/config

# Acceptable for small files, but harder to maintain at scale
- action: file-create
  path: /etc/sway/config
  content: |
    output * bg #000000 solid_color
```

**Quote octal modes.** File mode strings must be quoted to prevent YAML from interpreting them as integers:

```yaml
- action: file-permissions
  path: /etc/sudoers.d/admin
  mode: "0440"          # Correct -- quoted string
  # mode: 0440          # Wrong -- YAML reads this as integer 288
```

**Use `starforge inspect` before building.** Verify how your layers resolve without running a full build. The `--layers` flag shows which layer contributed each item:

```bash
starforge inspect kiosk packages --layers
starforge inspect kiosk partitions --layers
starforge inspect kiosk services -l
```

## Full Example: Kiosk Appliance

This example produces a kiosk appliance OS with 4 targets from 7 layers:

```yaml
name: kiosk-os
description: Kiosk appliance operating system

targets:
  device:
    layers:
      - ./layers/base
      - ./layers/graphical
      - ./layers/player

  device-dev:
    layers:
      - ./layers/base
      - ./layers/graphical
      - ./layers/player
      - ./layers/development

  installer:
    qemu:
      disks:
        - name: install-target
          size: 32G
    layers:
      - ./layers/installer-base
      - ./layers/installer

  installer-dev:
    qemu:
      disks:
        - name: install-target
          size: 32G
    layers:
      - ./layers/installer-base
      - ./layers/installer-dev
```

The project demonstrates several patterns:

- **Shared base**: The `base`, `graphical`, and `player` layers compose the production device image. The `device-dev` target adds a `development` layer on top for debug tools.
- **Separate installer**: The installer targets use entirely different layers (`installer-base`, `installer`), since the installer is a distinct minimal OS.
- **QEMU disks**: The installer targets define additional virtual disks for testing the installation flow in QEMU, simulating a target disk to write to.
- **Dev variants**: Both the device and installer have development counterparts, following a consistent pattern of adding a variant layer to an existing stack.

## See Also

- [Project Structure](../project-structure/) -- Target definitions, layer directories, and build artifacts.
- [Remote Layers](../remote-layers/) -- Sharing layers across projects via git, archives, or HTTP.
- [Building & Testing](../building-and-testing/) -- Building targets and inspecting resolved configuration.
- [Installer](../installer/) -- Building installer images for device provisioning.
