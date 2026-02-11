---
title: "Files & Directories"
weight: 7
---

StarForge provides nine actions for managing files in the target filesystem. All file operations run in **phase 4**, while permissions and ownership run in **phase 5** after all file operations are complete.

## Creating Directories

The `file-mkdir` action creates directories with optional ownership and permissions:

```yaml
- action: file-mkdir
  path: /data/uploads
  owner: app
  group: app
  mode: "0755"
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path of the directory to create. |
| `owner` | string | No | Owner username or UID. |
| `group` | string | No | Group name or GID. |
| `mode` | string | No | Octal permissions string (e.g., `"0755"`). **Must be quoted.** |

Parent directories are created automatically. See the [`file-mkdir` reference](../../actions/file-mkdir/) for full details.

## Creating Files

The `file-create` action puts files into the target filesystem. It supports three modes: layer path (file or directory), inline content, and external source.

### From a layer file or directory

Reference a file or directory in your layer directory with `layer_path`:

```yaml
# Copy a single file
- action: file-create
  layer_path: ./files/etc/pacman.conf
  path: /etc/pacman.conf

# Copy an entire directory tree
- action: file-create
  layer_path: ./files/etc
  path: /etc
```

When `layer_path` points to a directory, the entire tree is copied recursively. Files are merged into the destination -- existing files at the same path are overwritten, but other files in the directory are preserved.

### From inline content

Use the `content` field for short configuration files:

```yaml
- action: file-create
  path: /etc/hostname
  content: my-device

- action: file-create
  path: /etc/sudoers.d/wheel
  content: "%wheel ALL=(ALL:ALL) ALL\n"
  mode: "0440"
```

### From an external source

The `layer_source` field pulls files from a git repository or archive for a single step, overriding the layer directory:

```yaml
# Clone a git repo and copy files from it
- action: file-create
  layer_source: https://github.com/org/configs.git#v2.0
  layer_path: ./etc/myapp
  path: /etc/myapp

# Clone, build, then copy the output
- action: file-create
  layer_source: https://github.com/org/app.git#main
  layer_script: |
    make build
  layer_path: ./output/app.bin
  path: /usr/local/bin/app
```

Sources are cached in `.starforge/cache/sources/`. The `layer_script` field runs a build command inside the cloned source before `layer_path` is resolved.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path in the target filesystem. |
| `content` | string | Conditional | Inline file content. Mutually exclusive with `layer_path`. |
| `layer_path` | string | Conditional | Relative path to a file or directory in the layer. Mutually exclusive with `content`. |
| `mode` | string | No | Octal permissions (e.g., `"0644"`). **Must be quoted.** |
| `layer_source` | string | No | Git URL or archive URL for an external source. |
| `layer_script` | string | No | Build script to run inside the source before resolving `layer_path`. |

See the [`file-create` reference](../../actions/file-create/) for full details.

## Editing Files

The `file-edit` action modifies existing files using tagged content. The YAML tag on the `content` field determines the edit mode:

### Append and prepend

```yaml
# Add lines to the end of a file
- action: file-edit
  path: /etc/hosts
  content: !append |
    192.168.1.100  myserver

# Add lines to the beginning of a file
- action: file-edit
  path: /etc/hosts
  content: !prepend |
    # Managed by StarForge
```

### Insert before or after a pattern

The `!before` and `!after` tags insert content relative to lines matching a regular expression:

```yaml
# Insert a custom repo before the [extra] section
- action: file-edit
  path: /etc/pacman.conf
  content: !before
    pattern: "^\\[extra\\]"
    value: |
      [custom]
      Server = https://repo.example.com/$arch

# Add a DNS setting after the [network] header
- action: file-edit
  path: /etc/config.ini
  content: !after
    pattern: "^\\[network\\]"
    match: 1
    value: |
      dns = 8.8.8.8
```

The `match` field selects which occurrence of the pattern to target. For `!before` and `!after`, `0` (or omitted) means all occurrences; `1` means the first only.

### Truncate before or after a pattern

The `!truncate_before` and `!truncate_after` tags remove content around a matching line. The matched line itself is kept.

```yaml
# Keep only content from [main] onward
- action: file-edit
  path: /etc/config
  content: !truncate_before
    pattern: "^\\[main\\]"

# Remove everything after the END marker
- action: file-edit
  path: /etc/config
  content: !truncate_after
    pattern: "^# END"
```

### Summary of edit tags

| Tag | Type | Description |
|-----|------|-------------|
| `!append` | scalar | Append to end of file. |
| `!prepend` | scalar | Prepend to beginning of file. |
| `!before` | mapping | Insert before lines matching a regex. |
| `!after` | mapping | Insert after lines matching a regex. |
| `!truncate_before` | mapping | Remove all content before the match. |
| `!truncate_after` | mapping | Remove all content after the match. |

The file must already exist in the target filesystem before editing. Multiple edits to the same file are applied in order. See the [`file-edit` reference](../../actions/file-edit/) and the [YAML Reference](../../yaml-reference/) for full tag documentation.

## Copying Files Within the Target

The `file-copy` action copies files or directories within the built filesystem (not from the layer directory):

```yaml
- action: file-copy
  from_path: /etc/skel
  to_path: /home/player
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_path` | string | Yes | Source path inside the target filesystem. |
| `to_path` | string | Yes | Destination path inside the target filesystem. |

See the [`file-copy` reference](../../actions/file-copy/) for full details.

## Moving Files

The `file-move` action moves or renames files within the target filesystem:

```yaml
- action: file-move
  from_path: /etc/old.conf
  to_path: /etc/new.conf
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_path` | string | Yes | Current path inside the target filesystem. |
| `to_path` | string | Yes | New path inside the target filesystem. |

See the [`file-move` reference](../../actions/file-move/) for full details.

## Deleting Files

The `file-delete` action removes files or directories from the target filesystem:

```yaml
- action: file-delete
  path: /usr/share/doc
  recursive: true
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Path to delete inside the target filesystem. |
| `recursive` | bool | No | Remove directories and their contents. Default: `false`. |

See the [`file-delete` reference](../../actions/file-delete/) for full details.

## Creating Links

The `file-link` action creates symbolic or hard links:

```yaml
# Symbolic link (default)
- action: file-link
  from_path: /usr/share/zoneinfo/UTC
  to_path: /etc/localtime

# Hard link
- action: file-link
  from_path: /usr/bin/python3
  to_path: /usr/bin/python
  type: hard
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_path` | string | Yes | Link target (the file being pointed to). |
| `to_path` | string | Yes | Path of the link to create. |
| `type` | string | No | `symbolic` (default) or `hard`. |

See the [`file-link` reference](../../actions/file-link/) for full details.

## Setting Permissions

The `file-permissions` action sets file modes (chmod). Mode values **must be quoted** to prevent YAML from interpreting octal numbers as integers:

```yaml
- action: file-permissions
  path: /etc/sudoers.d/admin
  mode: "0440"

- action: file-permissions
  path: /opt/app
  mode: "0755"
  recursive: true
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Target path in the filesystem. |
| `mode` | string | Yes | Octal permissions string (e.g., `"0755"`, `"2775"`). **Must be quoted.** |
| `recursive` | bool | No | Apply to directory contents. Default: `false`. |

See the [`file-permissions` reference](../../actions/file-permissions/) for full details.

## Setting Ownership

The `file-ownership` action sets file owner and group (chown). At least one of `owner` or `group` is required:

```yaml
- action: file-ownership
  path: /data
  owner: data
  group: data

- action: file-ownership
  path: /home/player
  owner: player
  group: player
  recursive: true
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Target path in the filesystem. |
| `owner` | string | Conditional | Username or UID. At least one of `owner`/`group` required. |
| `group` | string | Conditional | Group name or GID. At least one of `owner`/`group` required. |
| `recursive` | bool | No | Apply to directory contents. Default: `false`. |

See the [`file-ownership` reference](../../actions/file-ownership/) for full details.

## Execution Order

All file operations run in **phase 4** in a fixed order:

1. **mkdir** -- Create directories.
2. **Layer copies** -- Copy directory trees from layers (`file-create` with `layer_path` pointing to a directory).
3. **Creates** -- Create individual files (`file-create` with `layer_path` pointing to a file, `content`, or `layer_source`).
4. **Edits** -- Apply `file-edit` modifications.
5. **Internal copies** -- Copy files within the target (`file-copy`).
6. **Moves** -- Move or rename files (`file-move`).
7. **Links** -- Create symbolic and hard links (`file-link`).
8. **Deletes** -- Remove files and directories (`file-delete`).

This ordering is deterministic and means you can rely on earlier operations completing before later ones. For example, you can create a file with `file-create` and then edit it with `file-edit` in the same layer.

## Permissions Run After Files

The `file-permissions` and `file-ownership` actions run in **phase 5**, after all phase 4 file operations are complete. This means ownership and permission changes are always applied to the final state of the filesystem, regardless of the order they appear in your layer YAML.

A common pattern is to create files in phase 4 and set their ownership in phase 5:

```yaml
# Phase 4: copy the player application into the target
- action: file-create
  layer_path: ./app
  path: /home/player/.local/share/player

# Phase 5: ensure the player user owns their home directory
- action: file-ownership
  path: /home/player
  owner: player
  group: player
  recursive: true
```

Within phase 5, ownership changes are applied before permission changes.
