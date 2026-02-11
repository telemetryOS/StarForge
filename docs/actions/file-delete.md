# file-delete

Remove files or directories from the target filesystem.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path to remove in the target filesystem. |
| `recursive` | bool | No | Remove directories and their contents recursively. Default: `false`. |

## Example

```yaml
# Remove a single file
- action: file-delete
  path: /etc/motd

# Remove a directory and its contents
- action: file-delete
  path: /usr/share/doc
  recursive: true
```

## Semantics

**Accumulate.** All delete operations from all layers are performed in order.

## Build Phase

Phase 4 (`files`). Deletes run last within the files phase, after all creates, edits, copies, moves, and links.

## Notes

- Deleting a non-existent path is not an error.
- Use `recursive: true` for directories. Attempting to delete a non-empty directory without `recursive: true` will fail.
