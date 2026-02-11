# systemd-socket

Create, enable, disable, or mask systemd socket units.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unit name. The `.socket` extension is added automatically if omitted. |
| `user` | string | No | Install as a user-level unit for this user. |
| `enable` | bool | No | Enable the unit. |
| `disable` | bool | No | Disable the unit. |
| `mask` | bool | No | Mask the unit. |
| `extends` | object | No | Create a drop-in override for an existing unit. |
| `layer_path` | string | No | Path to a unit file in the layer directory (or URL). |
| `unit` | map | No | `[Unit]` section fields. |
| `socket` | map | No | `[Socket]` section fields. |
| `install` | map | No | `[Install]` section fields. |

## Example

```yaml
- action: systemd-socket
  name: myapp
  enable: true
  unit:
    description: My App Socket
  socket:
    listen_stream: /run/myapp.sock
    socket_mode: "0660"
  install:
    wanted_by: sockets.target
```

## Modes, INI Conversion, and Drop-in Overrides

This action supports the same modes as [`systemd-service`](systemd-service.md): enable-only, disable-only, mask, inline definition, layer file, and drop-in override.

Field names in section maps use `snake_case` and are converted to systemd's `CamelCase` automatically (e.g., `listen_stream` becomes `ListenStream`). See [`systemd-service`](systemd-service.md) for full details on modes, `extends` syntax, INI field conversion, and the `!replace` tag for drop-in overrides. See also the [YAML Reference](../yaml-reference.md#systemd-ini-field-names) for the complete conversion table.

## Semantics

**Accumulate** for enable/disable/mask. **Replace-on-path** for unit files.

## Build Phase

Unit files written in phase 4 (`files`). Enable/disable/mask in phase 6 (`services`).
