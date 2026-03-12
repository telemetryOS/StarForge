---
title: "CLI Commands"
linkTitle: "Commands"
weight: 5
---

StarForge provides commands for creating, building, testing, and deploying OS images.

## Command Summary

| Command | Description |
|---------|-------------|
| [init](init/) | Create a new StarForge project |
| [build](build/) | Build overlay layers for a target |
| [run](run/) | Boot a target in QEMU |
| [write](write/) | Write a target to a device or disk image |
| [export](export/) | Export build artifacts as disk or partition images |
| [inspect](inspect/) | Inspect the resolved build context |
| [chroot](chroot/) | Enter the target filesystem interactively |
| [list](list/) | List targets defined in the project |
| [status](status/) | Show project and build status |
| [clean](clean/) | Remove build artifacts |

## Typical Workflow

```bash
# 1. Create a new project
starforge init my-os

# 2. Edit layers to define packages, files, services, etc.

# 3. Verify layer resolution before building
starforge inspect distribution

# 4. Build the target
starforge build distribution

# 5. Test in a QEMU virtual machine
starforge run distribution

# 6. Deploy to a physical device
starforge write distribution /dev/sdX
```

## Global Behavior

- **Project discovery.** StarForge looks for `starforge.yaml` by walking up from the current directory. All commands that operate on a project use this mechanism, so you can run them from any subdirectory.
- **Root access.** Most commands require root privileges. Operations like overlayfs mounts, arch-chroot, and writing to block devices cannot run unprivileged. StarForge will re-exec itself under `sudo` when needed.
- **Build tool vendoring.** Build tools (pacstrap, arch-chroot, etc.) are vendored automatically on first use and cached in `~/.local/share/starforge/`.
