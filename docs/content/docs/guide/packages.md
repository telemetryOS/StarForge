---
title: "Packages"
weight: 4
---

StarForge installs packages using `pacstrap`, the standard Arch Linux tool for bootstrapping a root filesystem. Packages are declared across layers using the `pacman-add` action and optionally pruned with `pacman-remove`.

## Adding Packages with `pacman-add`

The `pacman-add` action declares packages to install:

```yaml
- action: pacman-add
  packages:
    - base
    - linux
    - linux-firmware
    - sudo
    - networkmanager
    - openssh
```

Package names follow the standard Arch Linux naming conventions. Any package available in the configured pacman repositories can be listed here.

### Accumulation Across Layers

Packages accumulate across layers. Each layer adds to the combined package list without needing to repeat packages from earlier layers. This is the foundation of StarForge's layered composition model.

A base layer installs the core system:

```yaml
# layers/base/layer.yaml
- action: pacman-add
  label: Base system packages
  packages:
    - base
    - linux
    - linux-firmware
    - sudo
    - networkmanager
    - openssh
```

A desktop layer adds GUI packages on top:

```yaml
# layers/desktop/layer.yaml
- action: pacman-add
  label: Sway compositor and Wayland stack
  packages:
    - sway
    - swaybg
    - pipewire
    - pipewire-pulse
    - wireplumber
    - mesa
    - vulkan-intel
```

An application layer adds runtime dependencies:

```yaml
# layers/app/layer.yaml
- action: pacman-add
  label: Electron runtime dependencies
  packages:
    - nss
    - gtk3
    - alsa-lib
    - libnotify
    - xdg-utils

- action: pacman-add
  label: Docker container runtime
  packages:
    - docker
```

Multiple `pacman-add` steps within the same layer are also valid and are combined in order. This is useful for grouping packages by purpose with separate labels.

### Deduplication

During the Build phase, StarForge deduplicates the accumulated package list before passing it to `pacstrap`. If the same package appears in multiple layers or multiple steps, it is installed only once.

## Removing Packages with `pacman-remove`

The `pacman-remove` action removes packages that were added by earlier layers. This is useful when a specialized variant needs to exclude packages from a shared base:

```yaml
- action: pacman-remove
  packages:
    - nano
    - linux-firmware
```

Packages not found in the accumulated list are silently ignored. The `pacman-remove` action does not uninstall packages from an existing system -- it removes them from the package list before `pacstrap` runs.

### Practical Use Case

Consider a project with a full `device` target and a minimal `installer` target that shares the same base layer but does not need firmware or desktop tools:

```yaml
# starforge.yaml
targets:
  device:
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/app

  installer:
    layers:
      - ./layers/base
      - ./layers/installer
```

```yaml
# layers/installer/layer.yaml
- action: pacman-remove
  label: Strip unnecessary packages for installer
  packages:
    - linux-firmware
    - plymouth

- action: pacman-add
  label: Installer-specific packages
  packages:
    - dialog
    - parted
```

The installer target inherits all base packages, removes the ones it does not need, and adds its own.

## How Packages Are Installed

All package installation happens during **phase 1 (packages)** of the build pipeline. StarForge collects every `pacman-add` and `pacman-remove` step across all layers during the Collect phase, produces a final deduplicated package list, and passes it to `pacstrap` in a single operation.

This means:
- Package order does not matter. The final list is deduplicated regardless of the order packages were declared.
- You cannot install packages at different build stages. All packages are available starting from phase 2 onward.
- Package dependencies are resolved by pacman automatically, just as with a normal Arch Linux installation.

## Patterns and Recommendations

**Group by purpose.** Use separate `pacman-add` steps with labels for different package groups. This makes `starforge inspect` output more readable:

```yaml
- action: pacman-add
  label: Audio stack
  packages:
    - pipewire
    - pipewire-pulse
    - wireplumber

- action: pacman-add
  label: Bluetooth support
  packages:
    - bluez
    - bluez-utils
```

**Keep the base lean.** Install only essential packages in the base layer (`base`, `linux`, `linux-firmware`, `sudo`). Add feature-specific packages in dedicated layers so they can be excluded from minimal targets.

**Use `starforge inspect` to verify.** Before building, check the resolved package list:

```bash
starforge inspect device packages
```

This shows the final deduplicated list with layer provenance when the `--layers` flag is added:

```bash
starforge inspect device packages --layers
```

## See Also

- [`pacman-add`](../../actions/pacman-add/) -- Full action reference.
- [`pacman-remove`](../../actions/pacman-remove/) -- Full action reference.
- [Writing Layers](../writing-layers/) -- How accumulate semantics work across layers.
- [Building & Testing](../building-and-testing/) -- Inspecting resolved configuration before building.
