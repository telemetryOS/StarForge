---
title: "starforge clean"
weight: 10
---


Remove build artifacts.

## Usage

```
starforge clean <target> [scope]
starforge clean deps
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to clean, or `deps` for vendored dependencies. |
| `scope` | Optional scope: `cache`, `images`, or `disks`. Without a scope, removes everything. |

## Scopes

| Scope | Description |
|-------|-------------|
| *(none)* | Remove all build artifacts for the target (`.starforge/<target>/`). |
| `cache` | Remove only the overlay cache (`.starforge/<target>/cache/`). |
| `images` | Remove only partition images (`*.img` files) and rootfs. |
| `disks` | Remove extra QEMU disks (`.starforge/<target>/disks/`). |
| `deps` | Remove vendored dependencies (`~/.local/share/starforge/`). |

## Examples

```bash
# Remove all artifacts for a target
starforge clean device

# Remove only the cache (forces rebuild on next build)
starforge clean device cache

# Remove only partition images
starforge clean device images

# Remove extra QEMU disks
starforge clean device disks

# Remove vendored tools (re-downloaded on next build)
starforge clean deps
```

## Notes

- Cleaning the cache or all artifacts requires root access (cache directories contain root-owned files).
- Stale overlay mounts are cleaned up automatically before removal.
- `starforge clean deps` removes the `~/.local/share/starforge/` directory. Vendored tools will be re-downloaded on the next build.

## See Also

- [build](build/) -- Rebuild after cleaning
- [status](status/) -- Check build state
