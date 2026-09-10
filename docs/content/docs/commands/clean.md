---
title: "starforge clean"
weight: 10
---


Remove build artifacts.

## Usage

```
starforge clean <target> [scope]
starforge clean deps
starforge clean pacman
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to clean, or `deps`/`pacman` for global cleanup. |
| `scope` | Optional scope: `cache` or `artifacts`. Without a scope, removes everything for the target. |

## Scopes

| Scope | Description |
|-------|-------------|
| *(none)* | Remove all build artifacts for the target (`.starforge/<target>/`). |
| `cache` | Remove only the overlay cache (`.starforge/<target>/cache/`). |
| `artifacts` | Remove only sealed architecture-scoped disk artifacts and receipts. |
| `deps` | Remove vendored dependencies (`~/.local/share/starforge/`). |
| `pacman` | Remove persistent pacman package cache (`~/.local/state/starforge/pacman/`). |

## Examples

```bash
# Remove all artifacts for a target
starforge clean device

# Remove only the cache (forces rebuild on next build)
starforge clean device cache

# Remove only sealed artifacts
starforge clean device artifacts

# Remove vendored tools (re-downloaded on next build)
starforge clean deps

# Remove persistent pacman package cache
starforge clean pacman
```

## Notes

- VM-executor build state is user-owned, so cleaning it does not require root.
- A build directory created previously by the legacy native path may require one one-time privileged cleanup.
- `starforge clean deps` removes the `~/.local/share/starforge/` directory. Vendored tools will be re-downloaded on the next build.

## See Also

- [build](build/) -- Rebuild after cleaning
- [status](status/) -- Check build state
