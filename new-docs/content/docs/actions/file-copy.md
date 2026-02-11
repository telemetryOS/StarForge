---
title: "file-copy"
weight: 12
---


Copy files or directories within the target filesystem. This copies from one location in the target to another (not from the layer directory -- use [`file-create`](file-create/) for that).

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_path` | string | Yes | Source path within the target filesystem. |
| `to_path` | string | Yes | Destination path within the target filesystem. |

## Example

```yaml
# Copy a config file to a backup location
- action: file-copy
  from_path: /etc/pacman.conf
  to_path: /etc/pacman.conf.bak

# Copy a directory
- action: file-copy
  from_path: /etc/skel
  to_path: /home/newuser
```

## Semantics

**Accumulate.** All copies from all layers are performed in order.

## Build Phase

Phase 4 (`files`). Internal copies run after file creates and file edits.

## Notes

- Both paths are within the target filesystem, not the host or layer directory.
- To copy files from the layer directory into the target, use [`file-create`](file-create/) with `layer_path`.
- Parent directories for the destination are created automatically.
