---
title: "system-locale"
weight: 21
---


Set the system locale and optionally configure additional locales for generation.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `locale` | string | No | The primary locale (e.g., `en_US.UTF-8`). Sets `LANG` in `/etc/locale.conf`. |
| `locales` | list of strings | No | Additional locales to uncomment in `/etc/locale.gen` for generation. |

At least one of `locale` or `locales` should be specified.

## Example

```yaml
- action: system-locale
  locale: en_US.UTF-8
  locales:
    - en_US.UTF-8 UTF-8
    - en_GB.UTF-8 UTF-8
```

## Semantics

- **`locale`**: **Replace.** The last layer's value wins.
- **`locales`**: **Accumulate.** Locales from all layers are combined.

## Build Phase

Phase 2 (`sysconfig`). Writes `/etc/locale.conf` and modifies `/etc/locale.gen`, then runs `locale-gen`.

## Notes

- The `locale` field sets `LANG=<locale>` in `/etc/locale.conf`.
- The `locales` list adds entries to `/etc/locale.gen`. Each entry should include the encoding (e.g., `en_US.UTF-8 UTF-8`).
