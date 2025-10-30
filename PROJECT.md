# Star Forge Project

## Overview
This project manages Arch-based OS partition images for TelemetryTV. It provides tools to mount, chroot, resize, and manage multiple OS variants (development, production, staging, etc.) as partition image files.

The main command-line interface is `sf` (Star Forge), which dispatches to various management scripts.

## Project Structure

```
Star Forge/
├── config.yaml          # Configuration file defining OS targets and partitions
├── target-data/              # Partition image storage
│   ├── development/         # Star Forge images
│   │   ├── boot.img        # EFI/boot partition (FAT)
│   │   ├── root.img        # Root system partition (ext4)
│   │   ├── recovery_factory.img
│   │   ├── recovery.img
│   │   ├── var_log.img
│   │   └── data.img
│   └── [other-targets]/    # Other OS variants
├── bin/
│   └── sf                 # Main command dispatcher
├── scripts/
│   ├── common.sh           # Shared library (source in all scripts)
│   ├── sf_mount.sh        # Mount partition images
│   ├── sf_unmount.sh      # Unmount partitions
│   ├── sf_chroot.sh       # Enter chroot environment
│   ├── sf_status.sh       # Show mount/partition status
│   ├── sf_set-target.sh   # Switch between OS targets
│   ├── sf_clone-target.sh # Clone an OS target
│   └── sf_*.sh            # Other management scripts
└── mnt/                    # Mount point (created automatically)

```

## Configuration (config.yaml)

```yaml
# Current active target
current_target: development

# Target OS configurations
targets:
  - name: development
    description: Star Forge image
    target-data:
      - name: boot
        image: boot.img
        filesystem: vfat
        mount_point: boot
      - name: root
        image: root.img
        filesystem: ext4
        mount_point: .    # Root mount point
      # ... more partitions
```

### Key Concepts:
- **Target**: A complete OS variant (development, production, etc.)
- **Current Target**: The active target set in config, used by mount/unmount
- **Partitions**: Individual filesystem images that make up a target
- **Mount Point**: Where partition is mounted relative to mnt/

## Common Library (scripts/common.sh)

All scripts source this shared library. It provides:

### Variables:
- `PROJECT_DIR` - Project root directory
- `CONFIG_FILE` - Path to config.yaml
- `PARTITIONS_DIR` - Path to target-data/ directory
- `MOUNT_DIR` - Path to mnt/ directory
- `RED, GREEN, YELLOW, BLUE, NC` - Color codes for output

### Functions:
- `log_info()` - Green info messages
- `log_error()` - Red error messages
- `log_warn()` - Yellow warning messages
- `check_yq()` - Verify yq is installed
- `check_root()` - Verify running as root
- `check_config()` - Verify config file exists
- `get_current_target()` - Get current target from config
- `find_target_index()` - Find target index by name
- `validate_target()` - Check if target exists
- `list_targets()` - Display all available targets
- `get_partition_count()` - Get partition count for target
- `get_target_description()` - Get target description

### Usage Pattern:
```bash
#!/bin/bash
set -e
source "$(dirname "$0")/common.sh"

check_yq
check_root
check_config

TARGET=$(get_current_target)
# ... rest of script
```

## Scripts Reference

### mount.sh
**Purpose**: Mount all partition images for current target
**Usage**: `sf mount` (or `sudo ./scripts/sf_mount.sh` if not in dev shell)
**Dependencies**: yq, root privileges
**Process**:
1. Gets current target from config
2. Creates mnt/ directory
3. Mounts partition images directly using `mount -o loop`
4. Loopback devices are automatically created and managed by the kernel

**Mount ordering**: Uses path chunk count to ensure parent directories mount before children
- Root (`.`) = 0 chunks → mounts first
- `boot` = 1 chunk
- `var/log` = 2 chunks → mounts after var/

### unmount.sh
**Purpose**: Unmount all partitions
**Usage**: `sf unmount` (or `sudo ./scripts/sf_unmount.sh` if not in dev shell)
**Process**:
1. Finds all mounted filesystems under mnt/
2. Unmounts in reverse depth order (deepest first)
3. Loopback devices are automatically freed by the kernel
4. Removes empty directories

### chroot.sh
**Purpose**: Enter chroot environment (arch-chroot compatible)
**Usage**: `sf chroot [command]` (or `sudo ./scripts/sf_chroot.sh [command]` if not in dev shell)
**Features**:
- Mounts proc, sys, dev, dev/pts, dev/shm, run, tmp
- Mounts UEFI efivars if available (for bootloader work)
- Bind mounts resolv.conf for network access
- Uses unshare for PID namespace isolation
- Auto-cleanup on exit

**Interactive mode**: `sf chroot`
**Command mode**: `sf chroot "pacman -Syu"`

### status.sh
**Purpose**: Show current mount status and partition info
**Usage**: `sf status` (no root required)
**Displays**:
- Mount status (mounted/not mounted)
- Current target and description
- Partition list with sizes
- Mounted filesystems
- Summary with action hints

### set-target.sh
**Purpose**: View or change the current OS target
**Usage**:
- Show targets: `sf set-target`
- Switch target: `sf set-target production`

**Notes**: Cannot switch while partitions are mounted

### clone-target.sh
**Purpose**: Clone an existing target (config + all partition images)
**Usage**: `sf clone-target <source> <new> [description]`
**Example**: `sf clone-target development staging "Staging environment"`
**Process**:
1. Validates source exists and destination doesn't
2. Calculates total size
3. Prompts for confirmation
4. Copies all partition images
5. Adds new target to config

### resize-partition-image.sh
**Purpose**: Resize ext4 or FAT partition images
**Usage**: `sf resize-partition-image <partition_name|image_file> <new_size>`
**Examples**:
- By partition name: `sf resize-partition-image data 12G` (uses current target)
- By file path: `sf resize-partition-image target-data/development/data.img 12G`

**Features**:
- Auto-detects filesystem type
- Runs fsck before and after
- Shrink: filesystem first, then file
- Expand: file first, then filesystem
- Validates minimum size for shrinking
- Supports ext4 (via resize2fs) and FAT (via fatresize)

### load-installer.sh
**Purpose**: Load distribution images into an installer target's images partition
**Usage**: `sf load-installer <distribution-target> <installer-target>`
**Example**: `sf load-installer development development-installer`
**Process**:
1. Validates source is type "distribution" and destination is type "installer"
2. Checks installer has `images_partition` configured
3. Calculates total size of all distribution images
4. Verifies enough space on images partition
5. Mounts images partition temporarily
6. Copies all distribution partition images
7. Exports partition configuration to `partitions.yaml`
8. Unmounts and completes

**What gets copied:**
- All partition image files (boot.img, root.img, etc.)
- `partitions.yaml` - Configuration file containing partition metadata (name, image filename, filesystem, mount point, type, size in MB)

**Use case**: Prepare an installer OS with the distribution images it will deploy to target hardware. The installer can read `partitions.yaml` to create the correct partition table and then write each image to its corresponding partition.

### write-installer.sh
**Purpose**: Write an installer target to a USB drive or storage device
**Usage**: `sf write-installer <device>`
**Example**: `sf write-installer /dev/sdb`
**Process**:
1. Validates device exists and is a block device
2. Checks current target is type "installer"
3. Verifies all partition images exist
4. Wipes existing partition table
5. Creates GPT partition table
6. Creates partitions in order from config (last partition fills remaining space)
7. Determines optimal block size for the device
8. Writes each partition image using dd with optimal block size
9. Expands last partition's filesystem to fill available space
10. Syncs to disk

**Features**:
- Auto-detects optimal I/O block size for best write performance
- Automatically expands ext2/ext3/ext4 filesystems on last partition
- Handles standard and nvme/mmcblk device naming
- Progress indicators for write operations

**WARNING**: This command DESTROYS all data on the target device!

**Use case**: Create bootable USB installer media that can boot and deploy the distribution to target hardware

### image-sda.sh
**Purpose**: Create partition images from physical device
**Usage**: `sudo ./scripts/image-sda.sh`
**Note**: Helper script for initial image creation, not typically used in workflow

## Typical Workflows

### Working on Star Forge
```bash
# Initialize the development shell (elevates to root automatically)
sf

# View current status
sf status

# Mount the development OS (uses current_target from config)
sf mount

# Enter chroot to modify system
sf chroot

# Make changes, install packages, etc.
# (inside chroot)
pacman -S some-package
exit

# Unmount when done
sf unmount
```

### Creating a Production Variant
```bash
# Clone development to production
sf clone-target development production "Production OS"

# Switch to production target
sf set-target production

# Mount and customize
sf mount
sf chroot
# Make production-specific changes
exit
sf unmount
```

### Resizing Partitions
```bash
# Make sure partitions are unmounted
sf unmount

# Resize the data partition (by partition name - uses current target)
sf resize-partition-image data 20G

# Or specify full path
sf resize-partition-image target-data/development/data.img 20G
```

### Checking What's Mounted
```bash
# Quick status check (no root needed)
sf status

# Shows:
# - Current target
# - Mount status
# - Partition details
# - Mounted filesystems
```

### Preparing an Installer
```bash
# Make sure nothing is mounted
sf unmount

# Load distribution images into installer
sf load-installer development development-installer

# The installer's images partition now contains all the distribution images
```

### Writing Installer to USB
```bash
# Switch to installer target
sf set-target development-installer

# Write installer to USB drive (e.g., /dev/sdb)
# WARNING: This destroys all data on the device!
sf write-installer /dev/sdb

# The USB drive can now boot the installer OS
```

## Important Implementation Details

### Path Resolution
- All scripts calculate paths relative to project root
- `target-data/` directory contains subdirectories per target
- Images are located at: `target-data/<target>/<image>`

### Mount Ordering Algorithm
Uses path chunk count (number of `/` separators):
- `.` (root) = 0 chunks
- `boot` = 1 chunk
- `var/log` = 2 chunks

Sorts by chunks ascending to ensure parents mount before children.

### Loopback Device Management
- mount.sh uses `mount -o loop` to automatically create loopback devices
- Loopback devices are managed by the kernel with the autoclear flag
- unmount.sh automatically frees loopback devices when unmounting
- No manual loopback tracking or cleanup needed

### Target System
- Only one target can be active at a time (current_target in config)
- Each target has its own partition subdirectory
- Targets are completely isolated - no shared images
- mount.sh and chroot.sh use current_target automatically

### Error Handling
- All scripts use `set -e` for fail-fast behavior
- Common functions validate inputs before operations
- Scripts check for existing mounts before mounting
- Cleanup functions run on exit (via trap in chroot.sh)

### Dependencies
- **yq**: YAML parsing (install: `pacman -S go-yq`)
- **losetup**: Loopback device management (part of util-linux)
- **mount/umount**: Filesystem mounting (part of util-linux)
- **resize2fs, e2fsck**: ext4 resizing (part of e2fsprogs)
- **fatresize** (optional): FAT resizing
- **chroot, unshare**: Chroot environment (part of util-linux)

## Design Patterns

### Color-Coded Output
```bash
log_info "Success message"     # Green [INFO]
log_warn "Warning message"     # Yellow [WARN]
log_error "Error message"      # Red [ERROR]
```

### Validation Pattern
```bash
check_yq           # Verify dependencies
check_root         # Verify privileges
check_config       # Verify config exists
validate_target    # Verify target exists
```

### Target Lookup Pattern
```bash
TARGET=$(get_current_target)
target_index=$(find_target_index "$TARGET")
partition_count=$(get_partition_count "$target_index")
```

### Safe Cleanup Pattern
```bash
ACTIVE_RESOURCES=()

cleanup() {
    for resource in "${ACTIVE_RESOURCES[@]}"; do
        release_resource "$resource"
    done
}

trap cleanup EXIT
```

## Troubleshooting

### "Partitions appear to be already mounted"
**Cause**: Filesystems are still mounted at mnt/
**Solution**:
```bash
sf unmount  # Clean unmount
```

### "Image file not found"
**Cause**: Wrong target or images in wrong location
**Solution**:
```bash
sf status              # Check current target
sf set-target          # List available targets
ls target-data/development/       # Verify images exist
```

### "yq is not installed"
**Solution**: `sudo pacman -S go-yq`

### Orphaned Loopback Devices
**Note**: With the current implementation using `mount -o loop`, the kernel automatically cleans up loopback devices when filesystems are unmounted. Orphaned devices should not occur.
**If they do occur**: Simply unmount with `sf unmount`

### Can't Switch Targets
**Cause**: Current target is mounted
**Solution**: `sf unmount` first

## Future Considerations

### Adding New Partitions
1. Create image file in target directory
2. Add entry to target's partitions array in config
3. Specify name, image filename, filesystem type, mount_point

### Adding New Targets
1. Use `clone-target.sh` to copy existing target
2. OR manually create directory and add to config
3. Ensure image files exist in `target-data/<target>/`

### Modifying Common Functions
- Edit `scripts/common.sh`
- Changes affect all scripts immediately
- Test with multiple scripts to ensure compatibility

### Script Development Guidelines
- Always source common.sh first
- Use common functions for validation
- Follow color-coded logging pattern
- Add usage comments in header
- Handle cleanup properly (trap EXIT if needed)

## Notes for AI Agents

- This is an internal tool, not production deployment software
- Scripts prioritize clarity over optimization
- Error messages guide users to solutions
- All paths are relative to project root for portability
- YAML config is single source of truth for targets
- Scripts are idempotent where possible (safe to re-run)
- No network dependencies (except within chroot)
- Designed for Arch Linux but adaptable to other distros