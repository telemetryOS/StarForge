---
title: "system-keymap"
weight: 23
---


Set the console keyboard map.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `keymap` | string | Yes | Keymap name (e.g., `us`, `uk`, `de`). |

## Example

```yaml
- action: system-keymap
  keymap: us
```

## Semantics

**Replace.** If multiple layers set the keymap, the last layer wins.

## Build Phase

Phase 0 (`preinstall`) and Phase 2 (`sysconfig`).

In phase 0, the keymap is written to `/etc/vconsole.conf` before `pacstrap` runs (some packages need the keymap during installation). In phase 2, it is written again as part of full system configuration.

## Notes

- Keymap names correspond to files under `/usr/share/kbd/keymaps/`.
- Common values: `us`, `uk`, `de`, `fr`, `es`, `dvorak`.
