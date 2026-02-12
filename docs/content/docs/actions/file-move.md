---
title: "file-move"
weight: 13
---


Move or rename files within the target filesystem.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_path` | string | Yes | Current path within the target filesystem. |
| `to_path` | string | Yes | New path within the target filesystem. |

## Example

```yaml
# Rename a config file
- action: file-move
  from_path: /etc/default/grub
  to_path: /etc/default/grub.disabled

# Move a file to a different directory
- action: file-move
  from_path: /usr/share/app/config.yaml
  to_path: /etc/app/config.yaml
```

## Semantics

**Accumulate.** All moves from all layers are performed in order.

## Build Phase

Phase 4 (`files`). Moves run after internal copies.

## Notes

- Both paths are within the target filesystem.
- The source must exist at the time the move is executed.
- Parent directories for the destination are created automatically.
