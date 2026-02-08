# Star Forge - Technical Reference

## Scripts Reference

### bin/sf (Entry Point)
The main dispatcher. Handles privilege escalation and routes subcommands.
- If not root: displays elevation banner, re-invokes with `sudo -E`
- If no arguments and not in Star Forge shell: calls `enter_env()` from init.sh (launches interactive shell with custom prompt)
- If arguments: calls `setup_env()` then dispatches to `scripts/sf_<command>.sh`
- Sets `STAR_FORGE_SHELL=1` env var to prevent re-elevation

### scripts/init.sh
Two modes:
- **`enter_env()`**: Launches an interactive shell (bash/zsh/fish) with custom prompt showing target name and mount status. Sources completions.
- **`setup_env()`**: Lightweight setup for single-command execution. Downloads tools, creates directories, sets PATH.

Tool downloads (to `.tools/`):
- Arch bootstrap from archive.archlinux.org (provides pacman, arch-chroot)
- yq from GitHub releases (YAML processor)
- pv built from source (pipe viewer for progress)

### scripts/common.sh

#### Path Variables
```
PROJECT_DIR     → project root
CONFIG_FILE     → config.yaml path
TARGET_DATA_DIR → target-data/ directory
MOUNT_DIR       → mnt/ directory
TOOLS_DIR       → .tools/ directory
```

#### Key Functions

**Config reading:**
- `get_current_target()` → echoes target name or empty string
- `find_target_index(name)` → echoes numeric index or "-1"
- `get_partition_count(index)` → echoes count
- `get_target_type(index)` → "distribution" or "installer"
- `get_target_description(index)` → echoes description
- `get_images_partition(index)` → echoes images partition name (installer targets)

**Config writing:**
- `update_config(yq_expression)` → applies yq expression atomically (writes to temp, moves over original, preserves ownership/permissions)

**Validation:**
- `check_root()` → exits if not root
- `check_config()` → exits if config.yaml missing
- `validate_target(name)` → exits if target not in config, shows available targets
- `validate_target_type(type)` → validates "distribution" or "installer"
- `check_is_mounted()` → returns 0/1
- `check_not_mounted([msg])` → exits if mounted
- `check_not_current_target(name, [operation])` → exits if target is the active one
- `require_current_target()` → exits if no current target set, echoes target name

**User interaction:**
- `prompt_input(text, [default])` → echoes user input
- `prompt_password(text)` → echoes password (hidden input)
- `prompt_yesno(text)` → returns 0 for yes, 1 for no
- `confirm_or_exit([text])` → exits if user says no

**Display:**
- `print_header(title)` → centered title with horizontal rules
- `print_banner(title, [emoji])` → full-width banner
- `log_info(msg)` → plain output
- `log_error(msg)` → red [ERROR] prefix, to stderr
- `log_warn(msg)` → yellow text

**Utilities:**
- `relative_path(abs_path)` → converts to relative from PROJECT_DIR
- `is_boot_partition(mount_point)` → true if "boot" or "efi"
- `is_root_partition(mount_point)` → true if "." or empty
- `validate_timezone(tz)` → validates timezone string
- `normalize_timezone(tz)` → converts UTC offset to Etc/GMT format

**Virtual disk (QEMU):**
- `create_virtual_disk(target_name, dm_name)` → assembles partition images into `/dev/mapper/<dm_name>` via device-mapper. Sets globals: `LOOP_DEVICES[]`, `DM_DEVICE`, `GPT_FILE`
- `cleanup_virtual_disk(dm_name)` → tears down dm device, loop devices, GPT header file
- `setup_virtual_disk_cleanup_trap(dm_name)` → registers EXIT/INT/TERM trap handlers
- `get_ovmf_firmware_path()` → finds OVMF UEFI firmware on the system
- `setup_boot_overlay(target_name)` → mounts boot partition, replaces loader.conf and creates arch-qemu.conf entry from templates in `lib/bootd/qemu/`, unmounts. Saves originals for restore.
- `restore_boot_overlay()` → re-mounts boot partition, restores original configs, removes QEMU entry

### scripts/sf_mount.sh
Mounts all partitions for current target to `./mnt/`.
- Sorts by path depth (`.` first, nested paths later)
- Uses `mount -o loop` (kernel auto-manages loop devices)
- Creates intermediate directories as needed

### scripts/sf_unmount.sh
Unmounts in reverse depth order (deepest first).
- Finds mounts via `mountpoint` or by checking `/proc/mounts`
- Cleans up empty directories after unmount

### scripts/sf_chroot.sh
Enters arch-chroot inside mounted target.
- Requires target to be mounted first
- Supports interactive mode or single command: `sf chroot "pacman -Syu"`

### scripts/sf_create.sh
Interactive wizard (~488 lines). Walks through:
1. Target name and description
2. Target type (distribution/installer)
3. Partition layout (add partitions one by one)
4. Partition details: name, image filename, filesystem (ext4/vfat), mount point, type (linux/efi), size
5. Creates image files with `truncate` + `mkfs`
6. Updates config.yaml

### scripts/sf_run.sh / sf_run-serial.sh
Tests target in QEMU using virtual disk from `create_virtual_disk()`.
- **run**: graphical display, VGA output
- **run-serial**: serial console (`-nographic`, `console=ttyS0`)
- Both: 2GB RAM, 2 CPUs, KVM if available, VirtIO, UEFI boot, SSH port forwarding (2222:22)
- Boot overlay applied before QEMU starts, restored after exit

### scripts/sf_load-installer.sh
Copies distribution partition images into an installer target's images partition.
1. Validates source=distribution, dest=installer
2. Mounts installer's images partition temporarily
3. Copies all distribution .img files
4. Generates `partitions.yaml` metadata file
5. Unmounts

### scripts/sf_write-installer.sh
Writes installer target to a block device (USB drive).
1. Validates device exists and is block device
2. Creates GPT partition table
3. Writes each partition image with `dd` (optimal block size)
4. Expands last partition's filesystem to fill device
5. **Destructive operation** - prompts for confirmation

### scripts/sf_resize-partition-image.sh
Resizes ext4 or vfat partition images.
- Accepts partition name (uses current target) or direct file path
- Runs fsck before and after
- Shrink: resize filesystem first, then truncate file
- Expand: extend file first, then grow filesystem

### scripts/sf_clone.sh
Copies target config + all partition images to a new target name.

### scripts/sf_delete.sh
Removes a target's data directory and config entry. Cannot delete active target.

### scripts/sf_rename.sh
Renames target directory and updates config entry.

### scripts/sf_use.sh
Changes `current_target` in config.yaml. Cannot switch while mounted.

### scripts/sf_list.sh
Lists all targets with type, description, and current marker.

### scripts/sf_status.sh
Shows current target info, partition details, mount status, and disk usage.

### scripts/sf_export.sh / sf_import.sh
Export: copies partition image out (optionally compressed).
Import: copies image file in to replace a partition.

### scripts/sf_repair.sh
Validates target config against actual files, checks filesystem integrity.

## File Locations

| Path | Purpose |
|---|---|
| `config.yaml` | Target/partition definitions (source of truth) |
| `target-data/<target>/*.img` | Partition image files |
| `target-contrib/<target>/` | Scripts, configs, and files to be contributed into the target OS |
| `target-data/<target>/.gpt-header.img` | Temporary GPT header (created during QEMU runs) |
| `mnt/` | Active mount point tree |
| `.tools/arch-bootstrap/root.x86_64/` | Arch Linux bootstrap environment |
| `.tools/yq/yq` | yq binary |
| `.tools/pv/bin/pv` | pv binary |
| `lib/bootd/default/` | Default boot configs |
| `lib/bootd/qemu/` | QEMU boot config overrides (loader.conf, arch-qemu.conf) |
| `completions/sf.{bash,zsh,fish}` | Shell completions |

## Design Decisions

1. **Partition images over monolithic images** - Enables independent resizing, replacement, and fine-grained deployment
2. **YAML config as source of truth** - All target metadata in one file, manipulated via yq
3. **Self-bootstrapping tools** - No system-level package requirements beyond base Linux tools; arch-bootstrap, yq, and pv downloaded automatically
4. **Device-mapper for virtual disks** - Presents partition images as a single bootable disk to QEMU without copying data
5. **Boot overlay system** - QEMU-specific boot entries are applied temporarily and restored on exit, keeping partition images clean
6. **Atomic config updates** - `update_config()` writes to temp file then moves, preserving ownership and permissions
7. **Convention-based dispatch** - Adding `scripts/sf_foo.sh` automatically creates the `sf foo` command
8. **Filesystem-mirroring contrib** - `target-contrib/<target>/` files mirror their final OS paths, making deployment a simple recursive copy into the mounted target
