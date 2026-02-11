# file-mkdir

Create directories in the target filesystem with optional ownership and permissions.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path of the directory to create. |
| `owner` | string | No | Owner user name. |
| `group` | string | No | Owner group name. |
| `mode` | string | No | Directory permissions as an octal string. **Must be quoted.** |

## Example

```yaml
# Create a simple directory
- action: file-mkdir
  path: /opt/myapp

# Create with ownership and permissions
- action: file-mkdir
  path: /var/lib/myapp
  owner: myapp
  group: myapp
  mode: "0750"
```

## Semantics

**Accumulate.** All directory creations from all layers are performed in order.

## Build Phase

Phase 4 (`files`). Directory creation runs first within the files phase, before all other file operations.

## Notes

- Parent directories are created automatically (like `mkdir -p`).
- The `mode` field must be a **quoted string** (e.g., `"0755"`). Unquoted octal numbers are interpreted by YAML as integers. See the [YAML Reference](../yaml-reference.md#octal-numbers--always-quote-file-modes).
- If `owner` or `group` is specified, the user/group must exist (i.e., created by a `system-user` or `system-group` action in an earlier or same layer).
