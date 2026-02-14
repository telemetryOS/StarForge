---
title: "Project Structure"
weight: 1
---

A StarForge project is a directory containing a `starforge.yaml` file. This file defines the project name, an optional description, and one or more build targets. StarForge searches upward from the current directory to find this file, so you can run commands from any subdirectory within the project.

## The `starforge.yaml` File

The project file has three top-level fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Project name. Used in build output and artifact naming. |
| `description` | string | No | Human-readable description of the project. |
| `targets` | map | Yes | Named build targets. At least one is required. |

Here is a complete annotated example:

```yaml
# Project identity
name: kiosk-os
description: Kiosk appliance operating system

# Build targets -- each produces a different OS variant
targets:
  # Production device image
  device:
    layers:
      - ./layers/base          # Core system: partitions, packages, users
      - ./layers/graphical     # Sway compositor and Wayland stack
      - ./layers/player        # Application binary and kiosk service

  # Development variant with extra tools
  device-dev:
    layers:
      - ./layers/base
      - ./layers/graphical
      - ./layers/player
      - ./layers/development   # Git, editors, debug packages

  # Installer image (boots from USB, writes to internal disk)
  installer:
    qemu:                      # QEMU-specific configuration
      disks:
        - name: install-target
          size: 32G
    layers:
      - ./layers/installer-base
      - ./layers/installer
```

## Target Definitions

Each target is a named build profile with an ordered list of layers and optional configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `layers` | list | Yes | Ordered list of layer paths or URLs. |
| `args` | map | No | Variables that seed the initial scope for variable substitution. Values support `$NAME` / `${NAME}` env var expansion. |
| `default_env` | map | No | Default values for environment variables referenced in `args`. Used when the env var is not set. |
| `env` | map | No | Environment variables passed to `run` and `layer-run` scripts. Values support `${{ var }}` substitution against target `args`. |
| `qemu` | object | No | QEMU configuration for `starforge run` (additional disks, memory, etc.). |

Target `args` provide initial variable values that layers can reference via `${{ var_name }}` substitution and declare as required with `imports`. Arg values can be hardcoded strings or reference host environment variables:

```yaml
targets:
  production:
    args:
      version: "2.1.0"
      channel: stable
    env:
      APP_ENV: production
    layers:
      - ./layers/base
      - ./layers/app

  staging:
    args:
      version: "dev"
      channel: dev
    layers:
      - ./layers/base
      - ./layers/app
      - ./layers/dev-tools

  # Args from environment, with defaults
  ci:
    args:
      version: $CI_VERSION
      channel: $CI_CHANNEL
    default_env:
      CI_VERSION: "0.0.0-dev"
      CI_CHANNEL: dev
    layers:
      - ./layers/base
      - ./layers/app
```

## Layer Directories

Each layer is a directory containing a `layer.yaml` file. The directory can also hold any files or subdirectories that actions reference -- configuration files to copy into the target, scripts to run, and so on. There is no required structure beyond the `layer.yaml` file itself.

A typical layer directory:

```
layers/base/
├── layer.yaml                          # Layer definition (required)
├── files/
│   └── etc/
│       ├── NetworkManager/
│       │   └── conf.d/
│       │       └── connectivity.conf
│       ├── ssh/
│       │   └── sshd_config.d/
│       │       └── 10-hardening.conf
│       └── udev/
│           └── rules.d/
│               └── 60-readahead.rules
└── scripts/
    └── post-install.sh
```

The `file-create` action's `layer_path` field references files relative to the layer directory, so you can organize supporting files however you prefer. Many projects mirror the target filesystem structure under a `files/` subdirectory for clarity.

## Directory Layout Conventions

A complete project typically looks like this:

```
my-os/
├── starforge.yaml              # Project definition
├── .gitignore                  # Excludes .starforge/ build directory
├── layers/
│   ├── base/                   # Core system
│   │   ├── layer.yaml
│   │   ├── files/
│   │   └── scripts/
│   ├── desktop/                # GUI packages and compositor
│   │   ├── layer.yaml
│   │   └── files/
│   ├── app/                    # Application-specific config
│   │   └── layer.yaml
│   └── dev-tools/              # Development extras
│       └── layer.yaml
└── .starforge/                 # Build artifacts (generated, gitignored)
    ├── device/
    │   └── cache/
    └── device-dev/
        └── cache/
```

## Layer Ordering

Layers are processed in the order they appear in the target's `layers` list. This order matters because later layers can override, extend, or remove configuration set by earlier layers. The general pattern is:

1. **Base layer** -- partitions, core packages, system identity, core services
2. **Feature layers** -- desktop environment, networking stack, audio
3. **Application layers** -- application binaries, kiosk configuration
4. **Variant layers** -- development tools, debug settings

For detailed override semantics, see [Writing Layers](../writing-layers/).

## Remote Layer Sources

Layer entries in the `layers` list can be local paths or remote sources:

```yaml
layers:
  - ./layers/base                                          # local path
  - https://github.com/org/shared-layer.git#v2.0          # git repo
  - https://example.com/resources-v2.tar.gz                # archive
  - https://example.com/layers/desktop/                    # remote HTTP layer
  - ./layers/app                                           # local path
```

- **Git repositories** (`*.git#ref`) -- Shallow cloned. The ref (branch or tag) is optional; when omitted, the default branch is used.
- **Archives** (`.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.zip`) -- Downloaded and extracted with the top-level directory stripped.
- **Remote HTTP(S) layers** (any other URL) -- StarForge downloads `layer.yaml` from the URL and automatically fetches all files referenced by steps (`layer_path`, `script_path`, and `!include` paths).

Remote layers are fetched once and cached locally. Use `starforge clean <target>` to clear the cache and force re-fetching. For full details, see [Remote Layers](../remote-layers/).

## The `.starforge/` Build Directory

When you build a target, StarForge creates a `.starforge/` directory at the project root. This directory contains all build artifacts and should be added to `.gitignore` (the `starforge init` command does this automatically).

```
.starforge/
├── device/                     # Per-target build directory
│   ├── cache/                  # Overlay cache (one snapshot per phase)
│   │   ├── manifest.json       # Phase hashes for incremental builds
│   │   ├── phase-0/            # Overlay snapshot after phase 0
│   │   ├── phase-1/
│   │   └── ...
│   ├── root/                   # Final merged filesystem
│   └── images/                 # Partition images (after export)
├── cache/
│   ├── sources/                # Cached git repos and archives
│   ├── remote/                 # Cached remote HTTP layers
│   └── downloads/              # Cached individual file downloads
```

Vendored build tools (pacstrap, mkfs, etc.) are stored separately in `~/.local/share/starforge/`, not inside the project directory.

Build artifacts are separated by target, so building `device` and `device-dev` produces independent caches. Shared source caches (git repos, archives) are stored once and reused across targets.

## See Also

- [Writing Layers](../writing-layers/) -- Layer anatomy, steps, and override semantics.
- [Remote Layers](../remote-layers/) -- Full details on git, archive, and HTTP layer sources.
- [Multi-Target Projects](../multi-target-projects/) -- Using multiple targets to produce OS variants.
- [`starforge init`](../../commands/init/) -- Scaffolding a new project.
