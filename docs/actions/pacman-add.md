# pacman-add

Add packages to the target system using pacman. Packages are installed via `pacstrap` during the packages phase (phase 1).

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `packages` | list of strings | Yes | Package names to install. |

## Example

```yaml
- action: pacman-add
  packages:
    - base
    - linux
    - linux-firmware
    - sudo
    - openssh
    - networkmanager

# Compact form
- action: pacman-add
  packages: [vim, git, curl, htop]
```

## Semantics

**Accumulate.** Packages from all layers are combined into a single list. Duplicates are automatically removed at execute time. The final deduplicated list is passed to `pacstrap` as a single operation.

## Build Phase

Phase 1 (`packages`). All packages from all layers are installed together in one `pacstrap` invocation. After installation, `pacman-key --init` and `pacman-key --populate archlinux` are run to initialize the package signing keyring.

## Notes

- Package names must match the names in the Arch Linux repositories exactly.
- If a package has been removed from the repos, the build will fail during phase 1. Remove it from your layer and rebuild.
- Use [`pacman-remove`](pacman-remove.md) in a later layer to remove packages added by earlier layers.
- The `base` package is not included automatically. You must add it explicitly.
