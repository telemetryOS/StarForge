# file-link

Create symbolic or hard links in the target filesystem.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_path` | string | Yes | Link target -- the path the link points to. |
| `to_path` | string | Yes | Link path -- where the link is created. |
| `type` | string | No | Link type: `symbolic` (default) or `hard`. |

## Example

```yaml
# Symbolic link (default)
- action: file-link
  from_path: /usr/bin/python3
  to_path: /usr/bin/python

# Hard link
- action: file-link
  from_path: /etc/ssl/certs/ca-certificates.crt
  to_path: /etc/ssl/cert.pem
  type: hard
```

## Semantics

**Accumulate.** All link operations from all layers are performed in order.

## Build Phase

Phase 4 (`files`). Links run after moves and before deletes.

## Notes

- `from_path` is the target the link points to, `to_path` is where the link is created. This follows the convention: "link **from** target **to** location."
- Symbolic links can point to paths that don't exist yet (dangling links are allowed).
- Hard links require the target file to exist.
- The `type` field is a string, not a boolean. Use `symbolic` or `hard`.
