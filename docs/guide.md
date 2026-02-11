# Getting Started Guide

This guide walks you through building a custom Arch Linux OS image with StarForge, from project creation to booting your first image.

## Prerequisites

StarForge runs on **any Linux distribution**. Build tools like pacstrap, pacman, mkfs, and sgdisk are vendored automatically — downloaded from Arch Linux repositories on first use and cached in `~/.local/share/starforge/`.

- **Linux** (any distribution) with overlayfs support (standard on all modern kernels)
- **Root access** for build operations
- **Go 1.21+** to build StarForge from source
- **Internet access** on first run to download vendored dependencies
- **QEMU** for `starforge run` — the only host-installed dependency:

```bash
# Arch Linux
sudo pacman -S qemu-full

# Ubuntu/Debian
sudo apt install qemu-system-x86

# Fedora
sudo dnf install qemu-system-x86
```

## Building StarForge

```bash
cd StarForgeNext
go build -o starforge ./cmd/starforge
sudo cp starforge /usr/local/bin/
```

## Creating a Project

Use `starforge init` to scaffold a new project:

```bash
starforge init my-os
```

You'll be prompted for a description and target name (press Enter to accept the default `distribution`). This creates:

```
my-os/
├── starforge.yaml          # Project definition
├── .gitignore              # Excludes .starforge/ build dir
└── layers/
    └── base/
        └── layer.yaml      # Base layer configuration
```

## Project Structure

### starforge.yaml

The project file defines your build targets and which layers compose them:

```yaml
name: my-os
description: My custom Arch Linux distribution

targets:
  distribution:
    layers:
      - ./layers/base
      - ./layers/desktop
```

Each target is a named configuration that references an ordered list of layers. You can define multiple targets that share layers:

```yaml
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

Layers can also be remote — git repositories, archive URLs, or HTTP(S) URLs:

```yaml
targets:
  device:
    layers:
      - https://github.com/org/base-layer.git#v2.0          # git repo
      - https://example.com/resources.tar.gz                 # archive
      - https://example.com/layers/shared/                   # remote layer
      - ./layers/app
```

- **Git repos** (`*.git#ref`): Shallow cloned, ref is optional. When no ref is given, the default branch is used.
- **Archives** (`.tar.gz`, `.tar.xz`, `.zip`, etc.): Downloaded and extracted with the top-level directory stripped.
- **Remote layers** (any other URL): Downloads `layer.yaml` and automatically fetches all files referenced by steps (`layer_path`, `script_path`, and `!include` paths). The URL is treated as the layer directory root.

Remote layers are fetched once and cached locally in `.starforge/cache/`. Use `starforge clean <target>` to clear the cache and force re-fetching.

#### How Remote Layers Work

When a layer URL like `https://example.com/layers/base/` is used, StarForge:

1. Downloads `https://example.com/layers/base/layer.yaml`
2. Pre-fetches any `!include` files referenced in the layer
3. Resolves all includes (expanding them inline)
4. Scans all steps for relative `layer_path` and `script_path` references
5. Downloads each referenced file from the same base URL

For example, if a remote `layer.yaml` contains:

```yaml
steps:
  - action: file-create
    layer_path: ./etc/hostname
    path: /etc/hostname
  - action: run
    script_path: scripts/setup.sh
```

StarForge will fetch `https://example.com/layers/base/etc/hostname` and `https://example.com/layers/base/scripts/setup.sh` automatically.

Remote layers only support individual file references — directory copies via `layer_path` pointing to a directory are not supported for remote layers. Use git or archive layer sources instead for directory trees.

### Layer Directory

Each layer is a directory containing a `layer.yaml`. The directory can also contain any files or subdirectories that actions reference — there is no required structure beyond `layer.yaml` itself. For example:

```
layers/base/
├── layer.yaml
├── etc/
│   ├── hostname
│   ├── NetworkManager/
│   │   └── conf.d/
│   │       └── connectivity.conf
│   └── systemd/
│       └── journald.conf.d/
│           └── size-limit.conf
└── scripts/
    └── post-install.sh
```

The `file-create` action's `layer_path` field is a path relative to the layer directory, so you can organize your files however you like.

## Writing Layers

A layer's `layer.yaml` contains a list of steps. Each step specifies an action and its parameters. Steps can include an optional `label` field for readability:

```yaml
steps:
  - action: pacman-add
    label: Core system packages
    packages:
      - base
      - linux
```

Labels appear in build output and `starforge inspect --layers` but don't affect the build.

For details on YAML quoting rules and all custom tags, see the [YAML Reference](yaml-reference.md).

Large layers can use `!include` to split steps across multiple files:

```yaml
steps:
  - !include ./packages.yaml
  - !include ./services.yaml
  - action: system-hostname
    hostname: my-device
```

Included files are resolved relative to the including file's directory and can be nested up to 10 levels deep. See the [Actions Reference](actions/README.md#include--file-inclusion) for full details.

### Defining Partitions

The `partition-add` action defines your disk layout. Each partition needs a name, filesystem type, size, and mount point:

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
        size: 8G
        mount_point: /
        type: linux
```

Sizes accept K, M, G, or T suffixes. Append `+` to make a partition growable — it will expand to fill remaining space when writing to a device or exporting a disk image:

```yaml
      - name: data
        filesystem: ext4
        size: 256M+
        mount_point: /data
        type: linux
```

Partitions accumulate across layers. A later layer can add more partitions without redeclaring the entire layout:

```yaml
  # In a later layer — adds a logs partition
  - action: partition-add
    partitions:
      - name: logs
        filesystem: ext4
        size: 512M
        mount_point: /var/log
```

If a partition with the same name already exists, it is replaced in place (keeping its position). Use `after` to insert at a specific position:

```yaml
  # Insert a recovery partition after root
  - action: partition-add
    after: root
    partitions:
      - name: recovery
        filesystem: ext4
        size: 4G
        mount_point: /recovery
```

#### Removing Partitions

The `partition-remove` action removes a partition by name:

```yaml
  - action: partition-remove
    name: recovery
```

#### Modifying Partitions

The `partition-change` action modifies fields of an existing partition without replacing it entirely. Only specified fields are updated:

```yaml
  # Increase the root partition size
  - action: partition-change
    name: root
    size: 16G

  # Change the mount point
  - action: partition-change
    name: data
    mount_point: /storage
```

#### Partition Types

Partition types map to GPT type codes:
- `efi` — EFI System Partition
- `bios-boot` — BIOS boot partition
- `linux` — Linux filesystem (default)
- `home` — Linux home
- `swap` — Linux swap
- `raid` — Linux RAID
- `lvm` — Linux LVM
- `microsoft-basic` — Microsoft basic data
- `microsoft-reserved` — Microsoft reserved

### Installing Packages

The `pacman-add` action adds packages to be installed via `pacstrap`:

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

Packages accumulate across layers — a desktop layer can add GUI packages on top of the base:

```yaml
  # In layers/desktop/layer.yaml
  - action: pacman-add
    packages:
      - sway
      - swaybg
      - pipewire
      - wireplumber
```

### Removing Packages

The `pacman-remove` action removes packages added by earlier layers. This is useful when a later layer needs to exclude packages from the base:

```yaml
  - action: pacman-remove
    packages:
      - nano
      - linux-firmware
```

Packages not in the list are silently skipped (with a warning printed during collection).

### System Configuration

Set system identity with individual actions:

```yaml
  - action: system-hostname
    hostname: my-device

  - action: system-locale
    locale: en_US.UTF-8

  - action: system-timezone
    timezone: America/New_York

  - action: system-keymap
    keymap: us
```

These use replace semantics — if a later layer sets the hostname again, it overwrites the earlier value.

The `system-locale` action also supports generating additional locales:

```yaml
  - action: system-locale
    locale: en_US.UTF-8                     # default (LANG=)
    locales: [en_US.UTF-8, fr_FR.UTF-8]    # locale-gen list
```

### Creating Users

The `system-user` action defines user accounts. Later layers can modify users defined in earlier layers by referencing the same name:

```yaml
  # Base layer: create user
  - action: system-user
    name: admin
    uid: 1000
    groups: [wheel, video, audio]
    shell: /bin/bash
    password: changeme

  # Later layer: add groups without replacing existing ones
  - action: system-user
    name: admin
    groups: !add [docker]

  # Later layer: remove specific groups
  - action: system-user
    name: admin
    groups: !remove [audio]
```

Fields:
- `name` (required) — Username
- `groups` (optional) — List of group memberships. Use `!add` to append, `!remove` to remove, or plain list to replace.
- `shell` (optional, default: `/bin/bash`) — Login shell
- `password` (optional) — Plaintext password, hashed during build
- `uid` (optional) — Explicit UID (0 = auto-assign)
- `system` (optional, default: `false`) — Create as system user (no home directory)

### Creating Groups

The `system-group` action creates explicit groups:

```yaml
  - action: system-group
    name: data
    gid: 500
    system: true
```

Fields:
- `name` (required) — Group name
- `gid` (optional) — Explicit GID (0 = auto-assign)
- `system` (optional, default: `false`) — Create as system group

### Managing Services

Enable, disable, or mask systemd services using the `systemd-service` action:

```yaml
  - action: systemd-service
    name: NetworkManager
    enable: true

  - action: systemd-service
    name: sshd
    enable: true

  - action: systemd-service
    name: bluetooth
    disable: true
```

You can also create service units from inline definitions or layer files, and create drop-in overrides with `extends`. See the [systemd-service reference](actions/systemd-service.md) for all modes.

Other systemd unit types have matching actions: [`systemd-mount`](actions/systemd-mount.md), [`systemd-timer`](actions/systemd-timer.md), [`systemd-socket`](actions/systemd-socket.md), [`systemd-slice`](actions/systemd-slice.md), and [`systemd-target`](actions/systemd-target.md).

### Creating Files

The `file-create` action copies files or directories from your layer into the target filesystem:

```yaml
  - action: file-create
    layer_path: ./etc
    path: /etc
```

The `layer_path` field is relative to the layer directory — it can point to any file or directory within the layer. The `path` field is absolute within the target filesystem. Copying a directory merges its contents into the destination.

You can also create files with inline content:

```yaml
  # Create a file with inline content
  - action: file-create
    path: /etc/myapp/config.conf
    content: |
      [section]
      key = value
```

### Editing Files

The `file-edit` action modifies existing files. It supports inserting or truncating content. The edit mode is specified with a YAML tag on the `content` field:

```yaml
  # Append content to a file
  - action: file-edit
    path: /etc/myapp/config.conf
    content: !append |
      [extra]
      setting = value

  # Insert before a matching line
  - action: file-edit
    path: /etc/myapp/config.conf
    content: !before
      pattern: "^\\[extra\\]"
      value: |
        # Comment before extra section
```

Available tags: `!append`, `!prepend`, `!before`, `!after`, `!truncate_before`, `!truncate_after`. See the [`file-edit` reference](actions/file-edit.md) for all options.

### Copying Files Within the Target

The `file-copy` action copies files within the built filesystem:

```yaml
  - action: file-copy
    from_path: /etc/skel
    to_path: /home/player
```

### Moving Files

The `file-move` action moves or renames files within the target:

```yaml
  - action: file-move
    from_path: /etc/old.conf
    to_path: /etc/new.conf
```

### Deleting Files

The `file-delete` action removes files or directories:

```yaml
  - action: file-delete
    path: /usr/share/doc
    recursive: true
```

### Creating Links

The `file-link` action creates symbolic or hard links:

```yaml
  - action: file-link
    from_path: /usr/share/zoneinfo/UTC
    to_path: /etc/localtime
```

### Creating Directories

The `file-mkdir` action creates directories with optional ownership:

```yaml
  - action: file-mkdir
    path: /data/uploads
    owner: app
    group: app
    mode: "0755"
```

### Setting Permissions and Ownership

The `file-ownership` action sets owner and group (chown), while `file-permissions` sets the file mode (chmod):

```yaml
  # Set file ownership (chown)
  - action: file-ownership
    path: /etc/sudoers.d/admin
    owner: root
    group: root

  # Set file mode (chmod)
  - action: file-permissions
    path: /etc/sudoers.d/admin
    mode: "0440"

  # Both with recursive
  - action: file-ownership
    path: /opt/app
    owner: app
    group: app
    recursive: true

  - action: file-permissions
    path: /opt/app
    mode: "0755"
    recursive: true
```

**file-permissions** fields:
- `path` (required) — Target path in the filesystem
- `mode` (required) — Octal permissions string (e.g., `"0755"`)
- `recursive` (optional) — Apply to directory contents

**file-ownership** fields:
- `path` (required) — Target path in the filesystem
- `owner` (optional) — Username or UID (at least one of owner/group required)
- `group` (optional) — Group name or GID (at least one of owner/group required)
- `recursive` (optional) — Apply to directory contents

### Configuring the Bootloader

The `systemd-boot-install` action sets up systemd-boot:

```yaml
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
        options: rw quiet splash
```

The `root=UUID=...` parameter is injected automatically based on the root partition — you don't need to specify it.

### Running Scripts

The `run` action executes a script inside the chroot during build. You can reference a file or use inline content:

```yaml
  # File-based script
  - action: run
    script_path: scripts/post-install.sh

  # Inline script
  - action: run
    script: |
      sed -i '/\[multilib\]/,/Include/s/^#//' /etc/pacman.conf
      pacman -Sy

  # Run as a specific user
  - action: run
    script_path: scripts/user-setup.sh
    user: player
```

The `script_path` field is a path relative to the layer directory (or a URL). The `script` field allows inline scripts without a separate file. The `script` and `script_path` fields are mutually exclusive. The optional `user` field runs the script as the specified user instead of root. Scripts run in the final build phase after all other configuration is applied.

## Advanced Topics

### Systemd Drop-in Overrides

Drop-in overrides let you modify an existing systemd unit without replacing it. Use `extends` to specify the parent unit, and the `!replace` tag to clear directives before setting new values.

For example, to configure autologin on tty1:

```yaml
- action: systemd-service
  name: override.conf
  extends:
    service: getty@tty1
  service:
    exec_start: !replace "-/sbin/agetty --autologin player --noclear - $TERM"
```

This creates `/etc/systemd/system/getty@tty1.service.d/override.conf`:

```ini
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin player --noclear - $TERM
```

The `!replace` tag is essential here — without it, the new `ExecStart` would be *added* to the parent's list rather than replacing it. Use `!replace` whenever overriding list-type directives like `ExecStart`, `ExecStartPre`, or `Environment`.

Drop-in field names use `snake_case` and are converted to systemd's `CamelCase` automatically. See the [YAML Reference](yaml-reference.md#systemd-ini-field-names) for the full conversion rules.

### Splitting Layers with `!include`

The `!include` tag has two forms. The **scalar form** includes an entire file:

```yaml
steps:
  - !include ./packages.yaml
  - !include ./services.yaml
```

The **mapping form** includes a specific portion of a file using `yaml_path`:

```yaml
# shared.yaml
common:
  packages:
    - action: pacman-add
      packages: [base, linux]
  services:
    - action: systemd-service
      name: sshd
      enable: true

# layer.yaml — include only the packages from shared.yaml
steps:
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.packages
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.services
```

The `yaml_path` is dot-separated and supports mapping keys and sequence indices (e.g., `common.packages.0`). When an included sequence appears inside the `steps` list, its items are spliced in (flattened), not nested.

Includes can also reference URLs and nest up to 10 levels deep. See the [YAML Reference](yaml-reference.md#include) for full details.

### External Sources with `layer_source`

Any step can pull files from an external git repo or archive using `layer_source`. This overrides the layer directory for that single step. Combined with `layer_script`, you can build files from source before using them:

```yaml
# Clone a git repo and use its files
- action: file-create
  layer_source: https://github.com/org/configs.git#v2.0
  layer_path: ./etc/myapp
  path: /etc/myapp

# Download an archive, run a build script, and use the output
- action: file-create
  layer_source: https://github.com/org/app.git#main
  layer_script: |
    make build
  layer_path: ./output/app.bin
  path: /usr/local/bin/app
```

Sources are cached in `.starforge/cache/sources/`. Git repos are shallow cloned; archives are downloaded and extracted with the top-level directory stripped. `layer_script` (inline) and `layer_script_path` (file reference) are mutually exclusive and both require `layer_source`.

### User-Level Systemd Units

The `user` field on any systemd unit action installs the unit under a specific user's home directory instead of the system-wide location:

```yaml
# Create and enable a user-level service
- action: systemd-service
  name: pipewire
  user: player
  enable: true
  service:
    type: simple
    exec_start: /usr/bin/pipewire
  install:
    wanted_by: default.target
```

This installs to `/home/player/.config/systemd/user/pipewire.service` and enables it for that user. User units cannot use `extends` (drop-ins).

## Layering Strategy

A typical project uses layers to separate concerns:

```
layers/
├── base/          # Partitions, core packages, system config, base users
├── desktop/       # GUI packages, compositor, audio
├── kiosk/         # Application-specific config, autologin, lockdown
└── debug/         # SSH access, debug tools, development packages
```

**Base layer**: Define partitions, install core packages (`base`, `linux`, `linux-firmware`), set hostname/locale/timezone, create users, enable core services (networking, SSH, time sync).

**Feature layers**: Install additional packages, copy configuration files, enable services. These build on top of the base without repeating it.

**Variant layers**: Application-specific configuration. Combine with different feature layers to produce different OS variants via multiple targets.

### Multi-Target Projects

Use multiple targets to produce different OS variants from shared layers:

```yaml
# starforge.yaml
name: my-os
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

Each target builds independently with its own cache. The base layer is shared — changes to it invalidate all targets, while changes to `layers/kiosk` only affect the `kiosk` target.

Build and test individual targets:

```bash
starforge build minimal
starforge build kiosk
starforge run kiosk --serial
```

### Best Practices

- **One concern per layer**: Base layer for core system, feature layers for capabilities (desktop, networking, audio), variant layers for application-specific config.
- **Declare partitions early**: Define your full partition layout in the base layer. Later layers can modify individual partitions with `partition-change` without redeclaring the whole layout.
- **Use `!include` for large layers**: Split steps across files by concern (packages, services, files) to keep layers manageable.
- **Use labels**: Add `label` fields to steps for readable `starforge inspect --layers` output.
- **Prefer `file-create` with `layer_path`**: Keep configuration files as actual files in your layer directory rather than inlining them — it's easier to maintain and diff.
- **Quote octal modes**: File mode strings must be quoted (`"0755"`, not `0755`) to prevent YAML from interpreting them as integers.
- **Use `starforge inspect` before building**: Verify your layer resolution is correct without running a full build.

## Example: Complete Base Layer

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
      - name: data
        filesystem: ext4
        size: 256M+
        mount_point: /data
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

  - action: systemd-service
    name: systemd-timesyncd
    enable: true

  - action: systemd-service
    name: systemd-resolved
    enable: true

  - action: file-create
    layer_path: ./etc
    path: /etc

  - action: file-ownership
    path: /etc/sudoers.d/admin
    owner: root
    group: root

  - action: file-permissions
    path: /etc/sudoers.d/admin
    mode: "0440"
```

## Building

Build a target with:

```bash
starforge build distribution
```

The first build installs all packages and runs all phases. Subsequent builds use the overlay cache — only phases whose inputs changed are re-executed.

Force a full rebuild:

```bash
starforge build distribution --clean
```

## Inspecting the Build

Before building, verify how your layers resolve:

```bash
# Show everything
starforge inspect distribution

# Show just the package list
starforge inspect distribution packages

# Show partition layout with layer provenance
starforge inspect distribution partitions --layers

# See which layer set the hostname
starforge inspect distribution system -l

# Show enabled/disabled/masked services and default target
starforge inspect distribution services --layers

# Show all file operations
starforge inspect distribution files
```

Available concerns: `partitions`, `packages`, `groups`, `users`, `services`, `files`, `permissions`, `boot`, `system`, `scripts`. The `--layers` / `-l` flag shows which layer contributed each item and marks overridden values for replace-semantic fields.

## Testing in QEMU

Boot your build in a virtual machine:

```bash
starforge run distribution
```

SSH into the VM (in another terminal):

```bash
ssh -p 2222 localhost
```

Use `--serial` to attach a serial console for kernel boot messages:

```bash
starforge run distribution --serial
```

Use `--overlay` to persist changes made inside the VM across reboots:

```bash
starforge run distribution --overlay testing
```

The same overlay can be accessed via `starforge chroot distribution --overlay testing` for shell debugging.

## Deploying

### Write to a Device

Write directly to a USB drive or SD card:

```bash
starforge write distribution /dev/sdb
```

You will be prompted to confirm before data is destroyed.

### Export a Disk Image

Create a single bootable `.img` file:

```bash
starforge export distribution disk --size 16G --output ./release/my-os.img
```

The image can be flashed with `dd`:

```bash
sudo dd if=./release/my-os.img of=/dev/sdX bs=4M status=progress
```

### Export Partition Images

For OTA update systems or custom deployment:

```bash
starforge export distribution partitions --output ./release/
```

This produces individual files like `boot.img`, `root.img`, etc.

## Cleaning Up

```bash
# Remove all build artifacts for a target
starforge clean distribution

# Remove only the cache (force all phases to rebuild)
starforge clean distribution cache

# Remove only the partition images
starforge clean distribution images

# Remove vendored dependencies
starforge clean deps
```

## Command Reference

| Command | Description |
|---------|-------------|
| `starforge init [name]` | Create a new project |
| `starforge build <target> [--clean]` | Build disk images |
| `starforge write <target> <device>` | Write to a storage device |
| `starforge chroot <target> [--overlay <name>]` | Enter the built filesystem |
| `starforge run <target> [--serial] [--overlay <name>]` | Boot in QEMU |
| `starforge export <target> disk --size <size>` | Export full disk image |
| `starforge export <target> partitions` | Export partition images |
| `starforge inspect <target> [concern] [-l]` | Inspect resolved config |
| `starforge list` | List targets |
| `starforge status` | Show build state |
| `starforge clean <target> [scope]` | Remove build artifacts |
| `starforge clean deps` | Remove vendored dependencies |
