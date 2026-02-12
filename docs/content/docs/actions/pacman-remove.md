---
title: "pacman-remove"
weight: 2
---


Remove packages that were added by earlier layers. This action filters the accumulated package list before installation -- it does not uninstall packages from an existing system.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `packages` | list of strings | Yes | Package names to remove from the accumulated list. |

## Example

```yaml
# Base layer adds many packages
- action: pacman-add
  packages: [base, linux, linux-firmware, nano, vim]

# Minimal layer removes editors
- action: pacman-remove
  packages: [nano, vim]
```

## Semantics

**Remove.** Each listed package is removed from the accumulated package list. Packages that are not found in the list produce a warning but do not cause an error.

## Build Phase

This action modifies the package list during the Collect stage. The filtered list is then installed during phase 1 (`packages`).

## Notes

- This only affects the accumulated `pacman-add` list. It does not remove packages that are pulled in as dependencies.
- Removing a package that was never added produces a warning, not an error.
- The removal happens before deduplication and before `pacstrap` runs.
