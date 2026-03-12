---
title: "starforge chroot"
weight: 5
---


Enter the target filesystem interactively.

## Usage

```
starforge chroot [flags] <target> [-- command...]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to enter. |
| `command...` | Optional command to run instead of an interactive shell. |

## Flags

| Flag | Description |
|------|-------------|
| `--overlay <name>` | Use a named overlay for persistent changes across sessions. |

## Description

Mounts the built target's overlayfs layers and enters the filesystem using `arch-chroot`. By default, opens an interactive shell. If a command is provided after `--`, it is executed instead.

The target is built automatically if needed.

## Examples

```bash
# Interactive shell
starforge chroot device

# Run a single command
starforge chroot device -- pacman -Qi linux

# Use a named overlay for persistent changes
starforge chroot --overlay dev device
```

## Named Overlays

The `--overlay` flag creates a persistent copy of the partition images in `.starforge/<target>/overlays/<name>/`. Changes made during the chroot session are preserved across sessions. If the build hash changes (i.e., you run `starforge build` again), the named overlay is automatically recreated.

Overlay names must match: `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`

## Notes

- Requires root access (elevates automatically).
- Without `--overlay`, changes are discarded when the chroot exits.
- The chroot has full access to the built system, including package management.
- Useful for verifying installed packages, checking configuration files, testing service configurations, or running commands in the context of the built system.

## See Also

- [build](build/) -- Build disk images for a target
- [run](run/) -- Boot in QEMU for a full system test
- [inspect](inspect/) -- View resolved build context without entering chroot
