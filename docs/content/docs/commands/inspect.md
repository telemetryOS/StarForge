---
title: "starforge inspect"
weight: 7
---


Inspect the resolved build context for a target.

## Synopsis

```
starforge inspect <target> [concern] [--layers]
```

## Description

Show the final resolved state after all layers are collected for a target. This lets you verify how layers combine without running a build.

If no concern is specified, shows a summary of everything. Use a specific concern to focus on one aspect. Use `--layers` to see which layer contributed each item and to visualize overrides.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | Yes | Name of the target to inspect. |
| `concern` | No | Aspect to inspect (see table below). Defaults to showing all. |

## Concerns

| Concern | Description |
|---------|-------------|
| `partitions` | Disk partition layout with sizes, filesystems, and mount points |
| `packages` | Packages to install (deduplicated) |
| `groups` | Explicit group definitions from `system-group` |
| `users` | User accounts with shells and groups |
| `services` | Enabled, disabled, and masked systemd services (including user-level and default target) |
| `files` | All file operations: mkdir, create, edit, copy, move, link, delete |
| `permissions` | File ownership (chown) and mode (chmod) settings |
| `boot` | Bootloader configuration and entries |
| `system` | Hostname, locale, timezone, keymap, additional locales |
| `scripts` | Scripts to run during build (with labels and user context) |

## Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--layers` | `-l` | bool | `false` | Show layer provenance for each item. |

## Layer Provenance

With `--layers`, each item is annotated with the layer that contributed it. For fields with replace semantics (partitions, boot, system settings), overridden values from earlier layers are shown with strikethrough styling and marked as `(overridden)`, while the final value is marked as `(active)`.

## Examples

```bash
# Show everything
starforge inspect device

# Show only packages
starforge inspect device packages

# Show partition layout with layer provenance
starforge inspect device partitions --layers

# Show which layer set each system setting
starforge inspect device system -l

# Show all file operations
starforge inspect device files

# Show boot configuration
starforge inspect device boot --layers
```

## Notes

- Does not require root access (no build is performed).
- Does not require a prior build.

## See Also

- [build](build/) -- Build the target
- [list](list/) -- List available targets
- [status](status/) -- Check build state
