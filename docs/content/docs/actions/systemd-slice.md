---
title: "systemd-slice"
weight: 34
---


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
    MemoryMax: 2G
    CPUWeight: 100
```

## Modes and Drop-in Overrides

This action supports the same modes as [`systemd-service`](systemd-service/): enable-only, disable-only, mask, inline definition, layer file, and drop-in override.

Keys in section maps are written verbatim to the generated unit file -- use systemd's native CamelCase names. See [`systemd-service`](systemd-service/) for full details on modes, `extends` syntax, INI field names, and the `!replace` tag for drop-in overrides.

## Semantics

**Accumulate** for enable/disable/mask. **Replace-on-path** for unit files.

## Build Phase

Unit files written in phase 4 (`files`). Enable/disable/mask in phase 6 (`services`).
