# systemd-target

Set the system's default target or create/manage custom target units.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target` | string | Conditional | Set the default systemd target (e.g., `multi-user.target`, `graphical.target`). |
| `name` | string | Conditional | Target unit name. The `.target` extension is added automatically if omitted. |
| `user` | string | No | Install as a user-level unit. |
| `enable` | bool | No | Enable the target. |
| `disable` | bool | No | Disable the target. |
| `mask` | bool | No | Mask the target. |
| `layer_path` | string | No | Path to a unit file in the layer directory (or URL). |
| `unit` | map | No | `[Unit]` section fields. |
| `install` | map | No | `[Install]` section fields. |

Either `target` (set-default mode) or `name` (create/manage mode) is required.

## Examples

### Set default target

```yaml
- action: systemd-target
  target: graphical.target
```

### Create a custom target

```yaml
- action: systemd-target
  name: kiosk
  unit:
    description: Kiosk Target
    requires: multi-user.target
    after: multi-user.target
  install:
    aliases: kiosk.target
```

### Enable an existing target

```yaml
- action: systemd-target
  name: multi-user
  enable: true
```

## Modes

| Mode | Fields | Behavior |
|------|--------|----------|
| **Set-default** | `target` | Sets the system default target (`systemctl set-default`). |
| **Enable-only** | `name` + `enable: true` | Enables an existing target. |
| **Disable-only** | `name` + `disable: true` | Disables an existing target. |
| **Mask** | `name` + `mask: true` | Masks a target. |
| **Inline** | `name` + section maps | Creates a target unit file. |
| **Layer file** | `name` + `layer_path` | Creates a target unit file from layer. |

## Semantics

- **Set-default mode**: **Replace.** The last layer's `target` value wins.
- **Unit management modes**: Same as other systemd unit types. See [`systemd-service`](systemd-service.md).

Field names in section maps use `snake_case` and are converted to systemd's `CamelCase` automatically. See the [YAML Reference](../yaml-reference.md#systemd-ini-field-names) for the complete conversion table.

## Build Phase

Unit files written in phase 4 (`files`). Set-default and enable/disable/mask in phase 6 (`services`).
