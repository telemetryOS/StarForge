---
title: "systemd-service"
weight: 30
---


Create, enable, disable, or mask systemd service units.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unit name. The `.service` extension is added automatically if omitted. |
| `user` | string | No | Install as a user-level unit for this user (placed under `~/.config/systemd/user/`). |
| `enable` | bool | No | Enable the unit. |
| `disable` | bool | No | Disable the unit. |
| `mask` | bool | No | Mask the unit (symlink to `/dev/null`). |
| `extends` | object | No | Create a drop-in override for an existing unit. See [Drop-in overrides](#drop-in-overrides). |
| `layer_path` | string | No | Path to a unit file in the layer directory (or URL). |
| `unit` | map | No | `[Unit]` section fields. |
| `service` | map | No | `[Service]` section fields. |
| `install` | map | No | `[Install]` section fields. |

## Modes

The action operates in different modes depending on which fields are set:

| Mode | Fields | Behavior |
|------|--------|----------|
| **Enable-only** | `name` + `enable: true` | Enables an existing unit (no file created). |
| **Disable-only** | `name` + `disable: true` | Disables an existing unit. |
| **Mask** | `name` + `mask: true` | Masks a unit. |
| **Inline definition** | `name` + section maps | Creates a unit file from inline section maps. |
| **Layer file** | `name` + `layer_path` | Creates a unit file from a file in the layer directory. |
| **Drop-in override** | `name` + `extends` + content | Creates a drop-in file in `<parent>.d/`. |

## Examples

### Enable an existing service

```yaml
- action: systemd-service
  name: NetworkManager
  enable: true
```

### Inline service definition

```yaml
- action: systemd-service
  name: myapp
  enable: true
  unit:
    description: My Application
    after: network-online.target
    wants: network-online.target
  service:
    type: simple
    exec_start: /usr/bin/myapp --config /etc/myapp.conf
    restart: always
    restart_sec: 5
  install:
    wanted_by: multi-user.target
```

### Layer file

```yaml
- action: systemd-service
  name: myapp
  layer_path: ./units/myapp.service
  enable: true
```

### Drop-in override

```yaml
- action: systemd-service
  name: autologin.conf
  extends:
    service: getty@tty1
  service:
    exec_start: !replace "-/sbin/agetty --autologin player --noclear %I $TERM"
```

This creates `/etc/systemd/system/getty@tty1.service.d/autologin.conf`.

### User-level service

```yaml
- action: systemd-service
  name: myapp
  user: player
  enable: true
  unit:
    description: User application
  service:
    exec_start: /usr/bin/myapp
  install:
    wanted_by: default.target
```

This creates `/home/player/.config/systemd/user/myapp.service`.

## INI Field Name Conversion

Field names in section maps use `snake_case` and are automatically converted to systemd's `CamelCase`:

| YAML | Rendered |
|------|----------|
| `exec_start` | `ExecStart` |
| `wanted_by` | `WantedBy` |
| `restart_sec` | `RestartSec` |
| `type` | `Type` |

Known acronyms are preserved as uppercase: `CPUWeight`, `IODeviceWeight`, `IPAddressAllow`, `OOMScoreAdjust`, `TTYPath`, etc.

See the [YAML Reference](../yaml-reference/#systemd-ini-field-names) for the complete conversion table.

## Drop-in Overrides

The `extends` field creates a drop-in file for an existing unit:

```yaml
extends:
  service: getty@tty1    # → drop-in in getty@tty1.service.d/
  mount: home            # → drop-in in home.mount.d/
  timer: backup          # → drop-in in backup.timer.d/
```

The `extends` mapping has one key (the unit type) and one value (the unit name). The drop-in file is placed in `/etc/systemd/system/<unit>.d/<name>`.

Use `!replace` on values that need to be cleared before being set (the systemd drop-in pattern):

```yaml
service:
  exec_start: !replace "/usr/bin/new-command"
```

Renders as:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/new-command
```

## Semantics

**Accumulate** for enable/disable/mask operations. **Replace-on-path** for unit files (a later layer creating a unit at the same path replaces the earlier one).

## Build Phase

Unit files are written in phase 4 (`files`). Enable/disable/mask operations run in phase 6 (`services`).
