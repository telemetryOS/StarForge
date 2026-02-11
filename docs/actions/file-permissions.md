# file-permissions

Set file or directory permissions (chmod) in the target filesystem.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path in the target filesystem. |
| `mode` | string | Yes | Permissions as an octal string (e.g., `"0755"`). **Must be quoted.** |
| `recursive` | bool | No | Apply recursively to directory contents. Default: `false`. |

## Example

```yaml
# Set a script as executable
- action: file-permissions
  path: /usr/local/bin/startup.sh
  mode: "0755"

# Set directory permissions recursively
- action: file-permissions
  path: /var/lib/myapp
  mode: "0750"
  recursive: true
```

## Semantics

**Accumulate.** All permission changes from all layers are applied in order.

## Build Phase

Phase 5 (`permissions`). Permissions run after ownership changes within the permissions phase.

## Notes

- The `mode` field must be a **quoted string**. Unquoted `0755` is interpreted by YAML as the integer 493. See the [YAML Reference](../yaml-reference.md#octal-numbers--always-quote-file-modes).
- Use [`file-ownership`](file-ownership.md) to change file ownership.
