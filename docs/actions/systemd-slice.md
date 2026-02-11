# systemd-slice

Create, enable, disable, or mask systemd slice units for resource management.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unit name. The `.slice` extension is added automatically if omitted. |
| `user` | string | No | Install as a user-level unit for this user. |
| `enable` | bool | No | Enable the unit. |
| `disable` | bool | No | Disable the unit. |
| `mask` | bool | No | Mask the unit. |
| `extends` | object | No | Create a drop-in override for an existing unit. |
| `layer_path` | string | No | Path to a unit file in the layer directory (or URL). |
| `unit` | map | No | `[Unit]` section fields. |
| `slice` | map | No | `[Slice]` section fields. |
| `install` | map | No | `[Install]` section fields. |

## Example

```yaml
- action: systemd-slice
  name: app
  slice:
    memory_max: 2G
    cpu_weight: 100
```

## Modes, INI Conversion, and Drop-in Overrides

This action supports the same modes as [`systemd-service`](systemd-service.md): enable-only, disable-only, mask, inline definition, layer file, and drop-in override.

Field names in section maps use `snake_case` and are converted to systemd's `CamelCase` automatically (e.g., `memory_max` becomes `MemoryMax`, `cpu_weight` becomes `CPUWeight`). See [`systemd-service`](systemd-service.md) for full details on modes, `extends` syntax, INI field conversion, and the `!replace` tag for drop-in overrides. See also the [YAML Reference](../yaml-reference.md#systemd-ini-field-names) for the complete conversion table.

## Semantics

**Accumulate** for enable/disable/mask. **Replace-on-path** for unit files.

## Build Phase

Unit files written in phase 4 (`files`). Enable/disable/mask in phase 6 (`services`).
