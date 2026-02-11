# system-group

Create an explicit group in the target system.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Group name. |
| `gid` | int | No | Explicit GID. If omitted, assigned automatically. |
| `system` | bool | No | Create as a system group. Default: `false`. |

## Example

```yaml
- action: system-group
  name: myapp

- action: system-group
  name: kiosk
  gid: 1100
  system: true
```

## Semantics

**Accumulate.** Groups from all layers are created in order.

## Build Phase

Phase 3 (`users`). Groups are created before users, so users can reference them in their `groups` list.

## Notes

- Many groups are created automatically by packages or by `system-user` (each user gets a primary group with the same name). Use `system-group` only when you need a group that doesn't come from a package or user.
- Groups with `system: true` are created with `groupadd -r`.
