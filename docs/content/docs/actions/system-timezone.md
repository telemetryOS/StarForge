---
title: "system-timezone"
weight: 22
---


Set the system timezone.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `timezone` | string | Yes | Timezone name (e.g., `America/New_York`, `UTC`). |

## Example

```yaml
- action: system-timezone
  timezone: America/New_York
```

## Semantics

**Replace.** If multiple layers set the timezone, the last layer wins.

## Build Phase

Phase 2 (`sysconfig`). Creates a symlink from `/etc/localtime` to `/usr/share/zoneinfo/<timezone>`.

## Notes

- The timezone name must match a file under `/usr/share/zoneinfo/` in the Arch Linux packages.
- Use `starforge inspect` to see which layer set the timezone.
