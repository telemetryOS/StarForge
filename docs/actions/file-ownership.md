# file-ownership

Set file or directory ownership (chown) in the target filesystem.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path in the target filesystem. |
| `owner` | string | No | User name to set as owner. |
| `group` | string | No | Group name to set as group owner. |
| `recursive` | bool | No | Apply recursively to directory contents. Default: `false`. |

At least one of `owner` or `group` is required.

## Example

```yaml
# Change owner of a file
- action: file-ownership
  path: /home/player/.config
  owner: player
  group: player
  recursive: true

# Change group only
- action: file-ownership
  path: /var/log/myapp
  group: myapp
```

## Semantics

**Accumulate.** All ownership changes from all layers are applied in order.

## Build Phase

Phase 5 (`permissions`). Ownership changes run before permission changes within the permissions phase.

## Notes

- The user and group must exist in the target (created by `system-user` / `system-group` or by a package).
- `file-create` does not have owner/group fields. Use this action to set ownership after creating files.
- Ownership is resolved using the target's `/etc/passwd` and `/etc/group`, not the host's.
