---
title: "Systemd Units"
weight: 8
---

StarForge provides dedicated actions for every major systemd unit type: services, mounts, timers, sockets, slices, and targets. All share the same field structure and modes -- the only difference is which section map each unit type exposes (`service`, `mount`, `timer`, `socket`, or `slice`). This page covers all of them.

## Enabling and Disabling Services

The simplest use of `systemd-service` is toggling an existing unit. No unit file is created -- StarForge just records the enable/disable/mask operation for phase 6.

```yaml
# Enable services shipped by packages
- action: systemd-service
  name: NetworkManager
  enable: true

- action: systemd-service
  name: sshd
  enable: true

- action: systemd-service
  name: systemd-timesyncd
  enable: true

# Disable a service
- action: systemd-service
  name: bluetooth
  disable: true

# Mask a service (symlink to /dev/null, cannot be started)
- action: systemd-service
  name: systemd-homed
  mask: true
```

The `.service` extension is added automatically if omitted. You can enable and create a unit in the same step -- see the next section.

## Creating Unit Files

To create a new systemd service from scratch, provide the unit's section maps inline. The three section fields are `unit` (the `[Unit]` section), `service` (the `[Service]` section), and `install` (the `[Install]` section).

```yaml
- action: systemd-service
  name: myapp
  enable: true
  unit:
    Description: My Application
    After: network-online.target
    Wants: network-online.target
  service:
    Type: simple
    ExecStart: /usr/bin/myapp --config /etc/myapp.conf
    Restart: always
    RestartSec: 5
    Environment: "LOG_LEVEL=info"
  install:
    WantedBy: multi-user.target
```

This creates `/etc/systemd/system/myapp.service` containing:

```ini
[Unit]
Description=My Application
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/myapp --config /etc/myapp.conf
Restart=always
RestartSec=5
Environment=LOG_LEVEL=info

[Install]
WantedBy=multi-user.target
```

### INI Field Names

Keys in section maps are written verbatim to the generated unit file. Use systemd's native CamelCase names (e.g., `ExecStart`, `WantedBy`, `RestartSec`). The YAML section keys themselves (`unit`, `service`, `install`) are mapped to their canonical `[Unit]`, `[Service]`, `[Install]` headers automatically.

## Drop-in Overrides

Drop-in overrides modify an existing unit without replacing it entirely. This is the standard systemd pattern for customizing vendor-shipped units. Use the `extends` field to specify the parent unit.

```yaml
- action: systemd-service
  name: override.conf
  extends:
    service: getty@tty1
  service:
    ExecStart: !replace "-/sbin/agetty --autologin player --noclear %I $TERM"
```

The `extends` mapping has one key (the unit type) and one value (the unit name). This creates the file `/etc/systemd/system/getty@tty1.service.d/override.conf` with:

```ini
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin player --noclear %I $TERM
```

### The `!replace` Tag

The `!replace` tag is essential for drop-in overrides of list-type directives. In systemd, directives like `ExecStart`, `ExecStartPre`, and `Environment` are additive -- a drop-in file that sets `ExecStart=/new/command` would *add* to the existing list, not replace it. The systemd convention is to first set the directive to an empty string (clearing the list), then set the new value.

The `!replace` tag does exactly this. When StarForge renders a `!replace` value, it emits two lines: the directive with an empty value, then the directive with the new value.

Use `!replace` whenever you are overriding a list-type systemd directive in a drop-in. For scalar directives like `Type` or `Restart`, a plain value is sufficient since the drop-in simply overrides the parent's value.

### Drop-in for Other Unit Types

The `extends` key name determines the parent unit type:

```yaml
# Drop-in for a mount unit
- action: systemd-mount
  name: override.conf
  extends:
    mount: home
  mount:
    Options: "noatime,compress=zstd"

# Drop-in for a timer unit
- action: systemd-timer
  name: override.conf
  extends:
    timer: backup
  timer:
    OnCalendar: !replace "weekly"
```

## User-Level Services

The `user` field installs the unit under a specific user's home directory instead of the system-wide `/etc/systemd/system/` location.

```yaml
- action: systemd-service
  name: pipewire
  user: player
  enable: true
  service:
    Type: simple
    ExecStart: /usr/bin/pipewire
  install:
    WantedBy: default.target
```

This creates `/home/player/.config/systemd/user/pipewire.service` and enables it for that user. The unit is managed by the user's systemd instance (`systemctl --user`), not the system instance.

User-level units cannot use `extends` -- drop-in overrides are not supported for user units.

## Using `layer_path` for Unit Files

Instead of defining a unit inline, you can provide a complete unit file from your layer directory:

```yaml
- action: systemd-service
  name: myapp
  layer_path: ./units/myapp.service
  enable: true
```

The `layer_path` is relative to the layer directory. The file is copied directly into `/etc/systemd/system/myapp.service` (or the user unit directory if `user` is set). This is useful when you have complex unit files that are easier to maintain as standalone files.

## Other Unit Types

All systemd unit types share the same modes and features as `systemd-service`: enable-only, disable-only, mask, inline definition, layer file, drop-in override, and user-level installation. The only difference is which section map each type uses.

### systemd-mount

Creates `.mount` units. Uses the `mount` section instead of `service`.

```yaml
- action: systemd-mount
  name: var-log
  enable: true
  unit:
    Description: Mount /var/log as tmpfs
  mount:
    What: tmpfs
    Where: /var/log
    Type: tmpfs
    Options: "size=100M"
  install:
    WantedBy: local-fs.target
```

Mount unit names are derived from the mount path (e.g., `/var/log` becomes `var-log.mount`). See the [systemd-mount reference](../../actions/systemd-mount/) for full details.

### systemd-timer

Creates `.timer` units for scheduled tasks. Uses the `timer` section.

```yaml
- action: systemd-timer
  name: cleanup
  enable: true
  unit:
    Description: Run cleanup daily
  timer:
    OnCalendar: daily
    Persistent: true
  install:
    WantedBy: timers.target
```

Timer units typically activate a corresponding `.service` unit of the same name. See the [systemd-timer reference](../../actions/systemd-timer/) for full details.

### systemd-socket

Creates `.socket` units for socket activation. Uses the `socket` section.

```yaml
- action: systemd-socket
  name: myapp
  enable: true
  unit:
    Description: My App Socket
  socket:
    ListenStream: /run/myapp.sock
    SocketMode: "0660"
  install:
    WantedBy: sockets.target
```

When a connection arrives on the socket, systemd starts the corresponding `.service` unit. See the [systemd-socket reference](../../actions/systemd-socket/) for full details.

### systemd-slice

Creates `.slice` units for resource management (cgroups). Uses the `slice` section.

```yaml
- action: systemd-slice
  name: app
  slice:
    MemoryMax: 2G
    CPUWeight: 100
```

Place services under a slice by adding `Slice=app.slice` to their `[Service]` section. See the [systemd-slice reference](../../actions/systemd-slice/) for full details.

### systemd-target

The `systemd-target` action has two distinct modes.

**Set the default target** -- determines what systemd boots into:

```yaml
- action: systemd-target
  target: graphical.target
```

**Create a custom target unit** -- a grouping unit that other units can depend on:

```yaml
- action: systemd-target
  name: kiosk
  unit:
    Description: Kiosk Target
    Requires: multi-user.target
    After: multi-user.target
  install:
    Alias: kiosk.target
```

Custom targets are useful for defining a boot goal that aggregates multiple services. You can then set the custom target as the default:

```yaml
- action: systemd-target
  target: kiosk.target
```

See the [systemd-target reference](../../actions/systemd-target/) for full details.

## Execution Order

Systemd unit actions are split across two build phases:

- **Phase 4 (files)** -- Unit files are written to the target filesystem. This includes inline definitions, layer file copies, and drop-in override files.
- **Phase 6 (services)** -- Enable, disable, and mask operations are executed. Within this phase, operations run in a fixed order:
  1. Mask
  2. Enable
  3. Disable
  4. User-enable
  5. User-disable
  6. Set default target

This ordering means that if multiple layers mask and then enable the same unit, the enable takes effect (it runs after the mask). Disable runs after enable, so a later layer can disable a unit that an earlier layer enabled. The default target is always set last.

## See Also

- [systemd-service reference](../../actions/systemd-service/) -- Complete field reference and all modes.
- [systemd-boot-install reference](../../actions/systemd-boot-install/) -- Bootloader configuration (separate from unit management).
- [YAML Reference](../../yaml-reference/) -- Systemd INI field names and `!replace` tag syntax.
- [Bootloader](../bootloader/) -- Configuring systemd-boot entries and loader settings.
