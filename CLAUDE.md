# CLAUDE.md

## Project Overview

Star Forge is a Bash CLI tool for building, customizing, testing, and deploying custom Arch Linux OS images. It manages operating systems as collections of individual partition image files (boot.img, root.img, data.img, etc.) rather than monolithic ISOs.

Built for **TelemetryOS Edge** deployment.

## Architecture

```
bin/sf                    # Entry point dispatcher (auto-elevates to root)
scripts/
  common.sh               # Shared library sourced by all scripts (~960 lines)
  init.sh                 # First-run setup (downloads arch-bootstrap, yq, pv)
  sf_<command>.sh          # One script per subcommand (18 commands)
config.yaml               # YAML config: targets and their partition definitions
target-data/<target>/     # Partition image files (gitignored)
target-contrib/<target>/  # Scripts, configs, and files to be contributed into targets
mnt/                      # Runtime mount point (gitignored)
.tools/                   # Auto-downloaded tools: arch-bootstrap, yq, pv (gitignored)
lib/bootd/                # Boot config templates (default + qemu overrides)
completions/              # Shell completions (bash, zsh, fish)
```

## How It Works

- **Targets** are named OS configurations (e.g. "production") with a list of partitions
- **Two target types**: `distribution` (the OS) and `installer` (bootable USB that deploys a distribution)
- `config.yaml` is the single source of truth, manipulated via `yq`
- All scripts source `scripts/common.sh` for shared functions and variables
- `bin/sf` dispatches `sf <cmd>` to `scripts/sf_<cmd>.sh`
- Runs as root (auto-elevates via `sudo -E`)

## Commands (18 subcommands)

| Command | Purpose |
|---|---|
| `create` | Interactive wizard to create a new target |
| `clone` | Clone a target (config + all partition images) |
| `delete` | Delete a target |
| `rename` | Rename a target |
| `use` | Switch the active target |
| `list` | List all targets |
| `status` | Show current target status |
| `mount` | Mount partition images to `./mnt/` via loopback (distribution images only) |
| `mount --include-qemu-volumes` | Mount all images including QEMU-only volumes |
| `unmount` | Unmount all partitions (reverse depth order) |
| `chroot` | Enter arch-chroot in mounted target |
| `run` | Test target in QEMU (graphical, UEFI) |
| `run-serial` | Test target in QEMU (serial console) |
| `load-installer` | Copy distribution images into installer's images partition |
| `write-installer` | Write installer target to USB device (destructive) |
| `resize-partition-image` | Resize ext4/vfat partition images |
| `export` | Export a partition image |
| `import` | Import a partition image |
| `repair` | Repair/validate targets |

## Key Patterns

### Script structure
Every script follows this pattern:
```bash
#!/bin/bash
set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"
check_root
check_config
# ... command logic
```

### Config access (via yq)
```bash
TARGET=$(get_current_target)                    # Read current_target
target_index=$(find_target_index "$TARGET")     # Get array index
partition_count=$(get_partition_count "$target_index")
update_config ".current_target = \"$name\""     # Modify config atomically
```

### Important common.sh functions
- **Logging**: `log_info()`, `log_error()`, `log_warn()`
- **UI**: `print_header()`, `print_banner()`, `prompt_input()`, `prompt_yesno()`, `confirm_or_exit()`
- **Validation**: `check_root()`, `check_config()`, `validate_target()`, `check_is_mounted()`, `check_not_mounted()`
- **Config**: `get_current_target()`, `find_target_index()`, `get_partition_count()`, `get_target_type()`, `update_config()`
- **QEMU**: `create_virtual_disk()`, `cleanup_virtual_disk()`, `setup_boot_overlay()`, `restore_boot_overlay()`

### Mount ordering
Partitions are mounted by path depth (`.` first, then `boot`, then `var/log`). Unmounting is in reverse order.

By default, `sf mount` only mounts distribution images (boot, root, logs) and skips QEMU-only images (data, recovery, fallback-recovery). Use `--include-qemu-volumes` flag to mount all partitions.

### Virtual disk for QEMU
`create_virtual_disk()` in common.sh uses device-mapper to assemble partition images into a single bootable disk:
```
[GPT Header 1MB] [boot.img] [root.img] [...] [GPT Backup 34 sectors]
```
Each partition image is attached as a loop device, then dm-linear concatenates them. Changes write directly back to partition images.

## config.yaml Structure

```yaml
current_target: "production"
targets:
  - name: production
    description: "The production image for TelemetryOS Edge"
    type: distribution          # or "installer"
    # images_partition: images  # installer-only: which partition holds distro images
    partitions:
      - name: boot
        image: boot.img
        filesystem: vfat
        mount_point: boot       # "." means root mount
        type: efi               # or "linux"
```

## Development Notes

- **Language**: 100% Bash. No build system.
- **Tools are self-bootstrapped**: `init.sh` downloads arch-bootstrap, yq, and pv on first run to `.tools/`
- **No external dependencies at dev time** beyond standard Linux tools (util-linux, e2fsprogs, parted, qemu)
- **Error handling**: All scripts use `set -e`. QEMU scripts temporarily disable with `set +e` during cleanup.
- **Adding a new command**: Create `scripts/sf_<name>.sh`, it's auto-discovered by the dispatcher
- **Shell completions** in `completions/` must be updated manually when commands change
- **Boot overlay system**: `sf run` and `sf run-serial` temporarily swap boot configs with QEMU-specific versions from `lib/bootd/qemu/`, then restore originals on exit via trap handlers

## target-contrib Convention

`target-contrib/<target>/` holds files that mirror their final filesystem paths inside the target OS. To deploy configs into a mounted target:

```bash
sf mount
cp -a target-contrib/edge/* mnt/
sf chroot   # fix ownership/permissions if needed
sf unmount
```

Files are checked into git as the source of truth for target-specific OS configuration.

Each target's contrib folder may contain a `CLAUDE.md` with target-specific documentation (users, boot chain, hardware, deployment workflow, etc.).
