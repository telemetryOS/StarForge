---
title: "file-create"
weight: 10
---


Create files in the target filesystem from inline content or files in the layer directory. Supports both single files and directory trees.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path in the target filesystem. |
| `content` | string | Conditional | Inline file content. Mutually exclusive with `layer_path`. |
| `layer_path` | string | Conditional | Path to a file or directory in the layer directory (or a URL). Mutually exclusive with `content`. |
| `mode` | string | No | File permissions as an octal string. Default: `"0644"`. **Must be quoted.** |

Exactly one of `content` or `layer_path` is required.

## Examples

### Inline content

```yaml
- action: file-create
  path: /etc/hostname
  content: my-device

- action: file-create
  path: /etc/motd
  content: |
    Welcome to my custom OS!
    Built with StarForge.
```

### Single file from layer

```yaml
- action: file-create
  layer_path: ./files/etc/pacman.conf
  path: /etc/pacman.conf
```

### Directory from layer

When `layer_path` points to a directory, the entire directory tree is copied recursively:

```yaml
- action: file-create
  layer_path: ./files/etc
  path: /etc
```

This copies all files and subdirectories from `./files/etc/` in the layer directory into `/etc/` in the target.

### With custom mode

```yaml
- action: file-create
  path: /usr/local/bin/startup.sh
  mode: "0755"
  content: |
    #!/bin/bash
    echo "Starting up..."
```

### URL source

```yaml
- action: file-create
  layer_path: https://example.com/config/app.conf
  path: /etc/myapp/app.conf
```

## Semantics

**Replace-on-path.** If multiple layers create a file at the same path, the last layer's definition wins. The earlier file is replaced entirely.

Directory copies always accumulate -- files within the directory are merged, with later layers overwriting files at the same path within the tree.

## Build Phase

Phase 4 (`files`). Directory copies (layer copies) execute before single-file creates.

## Notes

- The `mode` field must be a **quoted string** (e.g., `"0644"`, `"0755"`). Unquoted octal numbers like `0755` are interpreted by YAML as the integer 493. See the [YAML Reference](../yaml-reference/#octal-numbers--always-quote-file-modes).
- Ownership of new files is inherited from the parent directory in the target filesystem. For example, a file created under `/home/player/` will be owned by `player:player` if that directory exists and is owned by that user. Use [`file-ownership`](file-ownership/) to set explicit ownership.
- `file-create` does not have `owner` or `group` fields -- use [`file-ownership`](file-ownership/) after creating the file.
- Parent directories are created automatically with ownership inherited from the closest existing ancestor.
- URL support for `layer_path` works for single files only, not directories. For directory trees from a remote source, use `layer_source` with a git or archive URL.
