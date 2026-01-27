# Star Forge

This file provides guidance to AI coding agents (Claude Code, Cursor, etc.) when working with code in this repository.

## Overview

CLI tool for building custom Arch-based operating systems. Manages complete OS images as "targets" (collections of partition images) that can be created, customized via chroot, tested in QEMU, and deployed to hardware via USB installers.

## Quick Reference

```bash
# Enter Star Forge shell (auto-elevates to root)
sf

# Common workflow
sf create my-os              # Create new target (interactive wizard)
sf mount && sf chroot        # Enter OS to customize
pacman -S nginx              # Install packages inside chroot
exit && sf unmount           # Exit and unmount
sf run                       # Test in QEMU (graphical)
sf run-serial                # Test in QEMU (serial console, exit: Ctrl+A X)

# Target management
sf list                      # List all targets
sf use <name>                # Switch active target
sf status                    # Show current target info
sf clone <src> <dest>        # Clone target
```

## Architecture

```
bin/sf                  # Main dispatcher - auto-elevates, routes to scripts
scripts/
  common.sh             # Shared library - MUST be sourced by all scripts
  sf_*.sh               # Subcommand implementations
lib/bootd/              # systemd-boot configs (default + qemu variants)
config.yaml             # Target definitions and current_target state
target-data/<target>/   # Partition images (boot.img, root.img, etc.)
```

### Key Components

- **`common.sh`**: Provides `check_root`, `check_config`, `get_current_target`, `find_target_index`, `validate_target`, `create_virtual_disk`, `cleanup_virtual_disk`, color logging (`log_info`, `log_error`, `log_warn`), and yq-based config functions.

- **Virtual Disk System**: `create_virtual_disk()` uses device-mapper to concatenate partition images into a bootable disk with GPT for QEMU. Creates loop devices, dm-linear table, writes partition table via parted.

- **Target Types**: `distribution` (deployable OS) and `installer` (bootable USB that deploys a distribution).

## Patterns & Conventions

### Script Structure
```bash
#!/bin/bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

check_root
check_config

TARGET=$(require_current_target)
target_index=$(find_target_index "$TARGET")
# ... script logic
```

### Mount Ordering
Partitions mount by path depth (fewer `/` separators first): `.` (root) -> `boot` -> `var/log`. Unmounting reverses this order.

### Adding New Scripts
1. Create `scripts/sf_<command>.sh`
2. Source `common.sh` first
3. Use validation functions before operations
4. Follow color-coded logging pattern
5. Script auto-registers via filename convention

### Config Updates
Use `update_config "yq expression"` for atomic YAML changes - handles temp files and preserves permissions.

## Dependencies

- **Required**: `yq` (go-yq), `util-linux`, `e2fsprogs`, `dosfstools`, `parted`, `qemu-full`, `edk2-ovmf`
- **Install**: `sudo pacman -S util-linux e2fsprogs dosfstools parted qemu-full go-yq edk2-ovmf`

## Gotchas

- Scripts require root (`check_root`) - `sf` auto-elevates via sudo
- Cannot switch targets while mounted - run `sf unmount` first
- QEMU uses boot overlay (swaps configs, restores on exit) - don't modify boot partition during run
- Loopback devices auto-managed by kernel with `mount -o loop` - no manual cleanup needed
- `config.yaml` is single source of truth - never hardcode target paths

## Reference

See `.claude/PROJECT.md` for detailed script documentation, implementation patterns, and troubleshooting guide.
