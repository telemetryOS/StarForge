---
title: "Examples"
weight: 10
---

This page walks through a complete StarForge project that builds a kiosk appliance operating system. It demonstrates how layers compose to produce multiple build targets from shared configuration.

## kiosk-os: A Production Kiosk Appliance OS

kiosk-os is an operating system for self-service kiosk terminals -- hardware units deployed in public spaces like museums, lobbies, and transit stations to run a Chromium-based kiosk browser. The project builds four distinct targets from seven reusable layers, producing both device images and USB installer images.

### Project Overview

The project root contains a single `starforge.yaml` that defines four targets:

```yaml
name: kiosk-os
description: Kiosk appliance operating system

targets:
  device:
    layers:
      - ./layers/base
      - ./layers/graphical
      - ./layers/app

  device-dev:
    layers:
      - ./layers/base
      - ./layers/graphical
      - ./layers/app
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

The four targets serve different purposes:

| Target | Layers | Purpose |
|--------|--------|---------|
| `device` | base + graphical + app | Production device image |
| `device-dev` | base + graphical + app + development | Development image with debug tools |
| `installer` | installer-base + installer | USB stick that installs the production image |
| `installer-dev` | installer-base + installer-dev | USB stick that installs the development image |

The `device-dev` target extends `device` by appending a single development layer. The installer targets attach a `qemu.disks` configuration so the installer can be tested in a virtual machine with a simulated 32G target disk.

To build a target:

```bash
starforge build device        # Production device image
starforge build device-dev    # Development device image
starforge build installer     # Production USB installer
```

### Directory Structure

```
kiosk-os/
  starforge.yaml
  layers/
    base/
      layer.yaml
      files/
        etc/
          NetworkManager/conf.d/connectivity.conf
          pipewire/pipewire.conf.d/fallback-sink.conf
          ssh/sshd_config.d/10-security.conf
          udev/rules.d/60-readahead.rules
          wireplumber/wireplumber.conf.d/
            auto-switch-bluetooth.conf
            no-pause-on-sink-removal.conf
    graphical/
      layer.yaml
      files/
        home/kiosk/.config/sway/config
    app/
      layer.yaml
    development/
      layer.yaml
    installer-base/
      layer.yaml
    installer/
      layer.yaml
    installer-dev/
      layer.yaml
```

Each layer directory contains a `layer.yaml` and an optional `files/` directory for configuration files that are too long to inline. The `files/` directory mirrors the target filesystem path structure for clarity, though the actual destination is specified by each step's `path` field.

### Layer Breakdown

#### base/ -- Foundation Layer

The base layer (294 lines) establishes the OS foundation: disk layout, packages, system identity, users, services, and system configuration. Every device target includes this layer.

**Partition layout.** The disk is divided into six partitions. The `data` partition uses the growable size syntax `256M+`, meaning it requires at least 256MB but expands to fill all remaining disk space:

```yaml
- action: partition-add
  label: Disk layout (boot, root, recovery, logs, data)
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
    - name: fallback-recovery
      filesystem: ext4
      size: 6G
      mount_point: /fallback-recovery
      type: linux
    - name: recovery
      filesystem: ext4
      size: 6G
      mount_point: /recovery
      type: linux
    - name: logs
      filesystem: ext4
      size: 512M
      mount_point: /var/log
      type: linux
    - name: data
      filesystem: ext4
      size: 256M+
      mount_point: /data
      type: linux
```

This layout separates logs and data onto their own partitions so a full log volume cannot fill the root filesystem. The dual recovery partitions enable A/B update strategies.

**Three user accounts with different roles.** The base layer creates a system user for data ownership, an interactive kiosk user, and an admin user:

```yaml
# System user -- owns /data, no login shell
- action: system-user
  label: Data service user
  name: data
  system: true
  shell: /usr/bin/nologin

# Kiosk user -- autologin, member of hardware groups
- action: system-user
  label: Kiosk application user
  name: kiosk
  groups: [wheel, video, render, seat, audio, input, data, docker, lp, network]
  shell: /bin/bash

# Admin user -- SSH access with password
- action: system-user
  label: Admin user
  name: admin
  groups: [wheel]
  shell: /bin/bash
  password: fiber-buffer-deploy-vault
```

The `kiosk` user has broad group membership for hardware access (video, audio, input) and container management (docker). The `admin` user has a password for SSH-based remote administration.

**Systemd configuration drop-ins.** Rather than editing default systemd config files, the base layer creates drop-in snippets for journald, logind, resolved, the system watchdog, and NTP:

```yaml
- action: file-create
  label: Journald size limit
  path: /etc/systemd/journald.conf.d/size-limit.conf
  content: |
    [Journal]
    SystemMaxUse=100M
    RuntimeMaxUse=50M
    MaxFileSec=1week

- action: file-create
  label: Logind disable idle/lid actions
  path: /etc/systemd/logind.conf.d/no-idle.conf
  content: |
    [Login]
    HandleLidSwitch=ignore
    HandleLidSwitchExternalPower=ignore
    HandleLidSwitchDocked=ignore
    IdleAction=ignore
    IdleActionSec=infinity
```

This pattern -- placing small config files in `.conf.d/` directories -- is idiomatic for systemd and avoids conflicts when upstream packages update their defaults.

**Boot entries with recovery options.** The [systemd-boot-install](actions/systemd-boot-install/) action configures three boot entries: a normal entry with Plymouth splash, a recovery entry, and a fallback recovery entry. The boot editor is disabled to prevent tampering on deployed devices:

```yaml
- action: systemd-boot-install
  label: Systemd-boot loader and entries
  loader:
    default: arch.conf
    timeout: 0
    editor: false
  entries:
    - name: arch.conf
      title: Kiosk OS
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet splash rootflags=noatime,commit=600 audit=0 noresume
    - name: recovery.conf
      title: Kiosk OS Recovery
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet rootflags=noatime,commit=600 noresume
```

**Autologin via systemd drop-in.** The base layer configures automatic login for the `kiosk` user by extending the `getty@tty1` service with a drop-in override:

```yaml
- action: systemd-service
  label: Autologin kiosk on tty1
  name: autologin.conf
  extends:
    service: getty@tty1
  service:
    ExecStart: !replace "-/sbin/agetty --noreset --noclear --noissue --skip-login --autologin kiosk - ${TERM}"
```

The `!replace` tag clears the existing `ExecStart` before setting the new value, which is required for systemd drop-in overrides where a directive must be emptied before reassignment. See [systemd-service](actions/systemd-service/) for details on the `extends` field.

**Data partition ownership.** The layer finishes by setting group-writable permissions with setgid on the data partition, so files created by any member of the `data` group inherit the correct ownership:

```yaml
- action: file-ownership
  label: Data partition ownership
  path: /data
  owner: data
  group: data

- action: file-permissions
  label: Data partition permissions (setgid)
  path: /data
  mode: "2775"
```

#### graphical/ -- Sway Compositor Layer

The graphical layer installs the Wayland compositor stack and configures a kiosk-mode Sway session. It sits between the base OS and the application layer, allowing the same graphical environment to be reused if the application changes.

**Cloning external repositories with `layer_source`.** The [file-create](actions/file-create/) action supports a `layer_source` field that clones a git repository into the target filesystem:

```yaml
- action: file-create
  label: sway-systemd repo
  layer_source: https://github.com/alebastr/sway-systemd.git
  path: /usr/local/share/sway-systemd

- action: file-permissions
  label: sway-systemd script permissions
  path: /usr/local/share/sway-systemd/src
  mode: "0755"
  recursive: true

- action: run
  label: Install sway-systemd targets
  script: |
    #!/bin/bash
    cp /usr/local/share/sway-systemd/units/*.target /usr/lib/systemd/user/
```

This three-step pattern -- clone, set permissions, run post-install -- is a common way to integrate third-party components that are not available as packages.

**Kiosk auto-start.** The graphical layer places a Sway config file from the layer's `files/` directory and creates a `.bash_profile` that launches Sway automatically when the `kiosk` user logs in on tty1:

```yaml
- action: file-create
  label: Sway kiosk configuration
  layer_path: ./files/home/kiosk/.config/sway/config
  path: /home/kiosk/.config/sway/config

- action: file-create
  label: Kiosk bash profile (auto-start sway)
  path: /home/kiosk/.bash_profile
  content: |
    [[ -f ~/.bashrc ]] && . ~/.bashrc

    if [ -z "$WAYLAND_DISPLAY" ] && [ "$(tty)" = "/dev/tty1" ]; then
        exec sway
    fi
```

Combined with the autologin drop-in from the base layer, this creates a seamless boot-to-compositor experience: the device boots, automatically logs in the `kiosk` user on tty1, and the bash profile launches Sway.

#### app/ -- Application Layer

The app layer installs the Chromium-based kiosk browser application and its runtime dependencies, Docker for container workloads, and user-level audio services.

**Full systemd service with watchdog.** The kiosk browser service unit demonstrates several advanced systemd features -- `Type: notify` with `WatchdogSec` for health monitoring, binding to the Sway session target, and multi-line `ExecStart` using YAML's `>-` folded scalar:

```yaml
- action: systemd-service
  label: Kiosk browser service
  name: kiosk-browser
  user: kiosk
  enable: true
  unit:
    Description: Kiosk Browser
    After: sway-session.target
    BindsTo: sway-session.target
  service:
    Type: notify
    NotifyAccess: all
    WatchdogSec: 30
    Restart: always
    RestartSec: 3
    ExecStart: >-
      /home/kiosk/.local/share/kiosk-browser/kiosk-browser
      --platform node-pro
      --enable-features=UseOzonePlatform
      --ozone-platform=wayland
    StandardOutput: journal
    StandardError: journal
  install:
    WantedBy: sway-session.target
```

The `WatchdogSec: 30` directive means systemd will restart the kiosk browser if it fails to send a heartbeat notification within 30 seconds. `BindsTo: sway-session.target` ensures the application stops if the compositor crashes and restarts when it comes back.

Both CamelCase and `snake_case` field names are accepted in systemd unit sections. This example uses CamelCase directly (`ExecStart`, `WantedBy`, `WatchdogSec`), while the guide examples use `snake_case` (`exec_start`, `wanted_by`). StarForge converts `snake_case` to CamelCase automatically; CamelCase names pass through unchanged. See the [YAML Reference](yaml-reference/#systemd-ini-field-names) for conversion rules.

**User-level PipeWire services.** Audio services run in the `kiosk` user's systemd session rather than as system services:

```yaml
- action: systemd-service
  label: Enable PipeWire for kiosk
  name: pipewire
  user: kiosk
  enable: true

- action: systemd-service
  label: Enable PipeWire PulseAudio for kiosk
  name: pipewire-pulse
  user: kiosk
  enable: true

- action: systemd-service
  label: Enable WirePlumber for kiosk
  name: wireplumber
  user: kiosk
  enable: true
```

The `user` field tells StarForge to enable these as user services (via `systemctl --user enable`) rather than system services.

**External build artifacts.** The layer copies the pre-built kiosk browser application from a sibling project directory using a relative `layer_path`:

```yaml
- action: file-create
  label: Kiosk browser application
  layer_path: ../../../App/out/kiosk-browser-linux-x64
  path: /home/kiosk/.local/share/kiosk-browser
```

This allows the OS build to incorporate artifacts from other build systems without embedding them in the layer repository.

#### development/ -- Development Tools Overlay

The development layer adds debugging and development tools on top of the production image. It is only included in the `device-dev` target.

**Merging user properties across layers with `!add`.** The base layer creates the `admin` user with `groups: [wheel]`. The development layer modifies that user without replacing the existing configuration -- `!add` appends to the group list rather than replacing it:

```yaml
- action: system-user
  label: Admin dev overrides
  name: admin
  shell: /usr/bin/fish
  no_password: true
  groups: !add [kiosk]
```

After both layers are processed, the `admin` user belongs to both `wheel` (from base) and `kiosk` (added by development). The `no_password: true` field removes the password requirement for sudo, and the shell is changed to `fish`.

**Running scripts as a specific user.** The `run` action accepts a `user` field to execute scripts in the context of a non-root user. This is important for tools like `nvm` that install into the user's home directory:

```yaml
- action: run
  label: Install nvm for admin
  user: admin
  script: |
    #!/bin/bash
    curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash

- action: run
  label: Install LazyVim for admin
  user: admin
  script: |
    #!/bin/bash
    git clone https://github.com/LazyVim/starter ~/.config/nvim
    rm -rf ~/.config/nvim/.git
```

Without `user: admin`, these scripts would run as root, and the installed files would end up in `/root` instead of `/home/admin`.

#### installer-base/ -- Minimal Installer OS

The installer-base layer builds a stripped-down OS for USB installation media. It uses a simpler partition layout and a minimal package set.

**Growable root partition.** The installer needs enough space for the OS plus the bundled device image. The `7G+` size means the root partition starts at 7GB minimum and expands to fill the USB drive:

```yaml
- action: partition-add
  label: Installer disk layout (boot + root)
  partitions:
    - name: boot
      filesystem: vfat
      size: 1G
      mount_point: /boot
      type: efi
    - name: root
      filesystem: ext4
      size: 7G+
      mount_point: /
      type: linux
```

**Install server and client.** The [install-server](actions/install-server/) action embeds the StarForge installer daemon into the image, and [install-client](actions/install-client/) adds a TUI client for interactive installation:

```yaml
- action: install-server
  label: Installer REST API server
  path: /images

- action: install-client
  label: Installer TUI client
```

The `path` field specifies where disk images will be stored on the installer USB.

#### installer/ and installer-dev/ -- Payload Bundling Layers

These single-step layers bundle a device image into the installer using [install-payload](actions/install-payload/):

```yaml
# installer/layer.yaml
steps:
  - action: install-payload
    label: Bundle device target
    target: device
    path: /images/device
```

```yaml
# installer-dev/layer.yaml
steps:
  - action: install-payload
    label: Bundle device-dev target
    target: device-dev
    path: /images/device-dev
```

The `target` field references another target from the same `starforge.yaml`. StarForge builds that target first (if not already cached), then embeds the resulting image into the installer at the specified path. This is how the `installer` target bundles the `device` image and the `installer-dev` target bundles the `device-dev` image.

---

## Common Patterns

### Autologin with a Systemd Drop-in

Override the default getty service to automatically log in a user on tty1. The `!replace` tag is required because systemd drop-ins must clear `ExecStart` before setting a new value:

```yaml
- action: systemd-service
  name: override.conf
  extends:
    service: getty@tty1
  service:
    ExecStart: !replace "-/sbin/agetty --autologin kiosk --noclear - $TERM"
```

This creates a drop-in file at `/etc/systemd/system/getty@tty1.service.d/override.conf`.

### Building User Accounts Across Layers

User properties can be progressively built across layers. The base layer creates the user, and subsequent layers modify it:

```yaml
# base/layer.yaml -- create user with initial groups
- action: system-user
  name: operator
  groups: [wheel, video]
  shell: /bin/bash
  password: changeme

# desktop/layer.yaml -- add desktop groups
- action: system-user
  name: operator
  groups: !add [audio, input, seat]

# app/layer.yaml -- add application-specific group
- action: system-user
  name: operator
  groups: !add [docker]
```

After all three layers, `operator` belongs to `wheel`, `video`, `audio`, `input`, `seat`, and `docker`. Using `!add` ensures each layer only appends to the list without overwriting groups set by earlier layers.

### Splitting a Large Layer with !include

When a layer grows beyond a few hundred lines, split it into focused files using the `!include` tag:

```yaml
# layer.yaml
steps:
  - !include packages.yaml
  - !include users.yaml
  - !include services.yaml
  - !include files.yaml
```

Each included file contains one or more steps. The `!include` tag inlines the content at parse time, so the result is identical to a single large file.

### Sharing Steps Across Targets

Place shared configuration in a `shared/` directory and include it from multiple layers:

```yaml
# layers/server/layer.yaml
steps:
  - !include ../../shared/base-packages.yaml
  - !include ../../shared/ssh-hardening.yaml
  - action: pacman-add
    label: Server-specific packages
    packages:
      - nginx
      - postgresql
```

```yaml
# layers/workstation/layer.yaml
steps:
  - !include ../../shared/base-packages.yaml
  - !include ../../shared/ssh-hardening.yaml
  - action: pacman-add
    label: Workstation packages
    packages:
      - firefox
      - code
```

Both layers inherit the same base packages and SSH hardening without duplication.

### Kiosk Lockdown Pattern

Combine several techniques to lock down a device for unattended kiosk use:

```yaml
# Disable boot editor to prevent manual kernel parameter edits
- action: systemd-boot-install
  loader:
    default: kiosk.conf
    timeout: 0
    editor: false
  entries:
    - name: kiosk.conf
      title: Kiosk OS
      linux: /vmlinuz-linux
      initrd: /initramfs-linux.img
      options: rw quiet splash audit=0

# Mask rescue and emergency targets to prevent recovery shell access
- action: systemd-service
  name: rescue
  mask: true

- action: systemd-service
  name: emergency
  mask: true

# Restrict sudo -- kiosk user gets NOPASSWD for managed commands only
- action: file-create
  path: /etc/sudoers.d/kiosk
  content: "kiosk ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart kiosk-browser\n"
  mode: "0440"

# Disable idle actions so the device never sleeps
- action: file-create
  path: /etc/systemd/logind.conf.d/no-idle.conf
  content: |
    [Login]
    HandleLidSwitch=ignore
    IdleAction=ignore
    IdleActionSec=infinity
```

### Development Overlay Pattern

Add a development layer that overrides production restrictions without modifying the production layers:

```yaml
# development/layer.yaml
steps:
  # Add development tools
  - action: pacman-add
    label: Development packages
    packages:
      - base-devel
      - git
      - vim
      - htop
      - curl

  # Remove password requirement for the admin user
  - action: system-user
    name: admin
    no_password: true
    groups: !add [docker]

  # Install user-specific tooling as that user
  - action: run
    label: Install nvm
    user: admin
    script: |
      #!/bin/bash
      curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash

  # Fix home directory ownership after modifications
  - action: file-ownership
    path: /home/admin
    owner: admin
    group: admin
    recursive: true
```

This layer is appended only to the development target, leaving the production target untouched. The pattern of `run` with `user` followed by `file-ownership` with `recursive: true` ensures files created by the script are correctly owned.
