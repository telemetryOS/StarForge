---
title: "system-hostname"
weight: 20
---


Set the system hostname.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `hostname` | string | Yes | The hostname to set. |

## Example

```yaml
- action: system-hostname
  hostname: my-device
```

## Semantics

**Replace.** If multiple layers set the hostname, the last layer wins.

## Build Phase

Phase 2 (`sysconfig`). Writes `/etc/hostname`.

## Notes

- Use `starforge inspect` to see which layer set the hostname (the history is tracked).
