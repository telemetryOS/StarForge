# Automated Installer OS Implementation Plan

## Overview

Transform the `development-installer` target into a fully automated installer OS that boots, auto-logs in, and installs the TelemetryTV Star Forge to the target device with minimal user interaction.

## Current State

- **Mounted Target**: `development-installer` at `./mnt/`
- **Images Available**:
  - `boot.img` (1GB, vfat, EFI)
  - `root.img` (4GB, ext4)
  - `recovery.img` (4GB, ext4)
  - `recovery_factory.img` (4GB, ext4)
  - `var_log.img` (1GB, ext4)
  - `data.img` (13GB, ext4)
- **Configuration**: `/images/partitions.yaml` defines partition layout
- **Total Distribution Size**: 27GB

---

## Architecture Overview

### Boot Sequence

1. **System Boot** → GRUB/systemd-boot loads installer OS
2. **Auto-login** → Getty auto-logs in as `installer` user on tty1
3. **Auto-start** → Shell profile launches installation script
4. **Device Detection** → Identifies target installation device
5. **Warning & Countdown** → 15-second abort window
   - No keypress: Continue with auto-detected device
   - Keypress: Enter interactive mode (device selection or abort)
6. **Installation** → Automated disk partitioning and image writing
7. **Bootloader Setup** → Install and configure boot system
8. **Completion** → Power off or reboot

---

## Implementation Components

### 1. User & Auto-login Setup

**Task**: Create installer user and configure automatic login

**Files to Create/Modify**:
- Create user: `useradd -m -s /bin/bash installer`
- Lock account: `passwd -l installer`
- Auto-login config: `/etc/systemd/system/getty@tty1.service.d/autologin.conf`

**Auto-login Configuration**:
```ini
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin installer --noclear %I $TERM
```

---

### 2. Main Installation Script

**Location**: `/usr/local/bin/auto-installer.sh`

**Responsibilities**:
- Display branded banner and system info
- Detect target installation device
- Show warning with 15-second countdown
- Handle abort on any keypress
- Execute installation workflow
- Handle errors gracefully
- Log all operations

**Script Structure**:
```bash
#!/bin/bash
set -e

# Configuration
IMAGES_DIR="/images"
PARTITIONS_YAML="$IMAGES_DIR/partitions.yaml"
LOG_FILE="/var/log/installer.log"

# Functions
log_info()    # Green output + log file
log_warn()    # Yellow output + log file
log_error()   # Red output + log file
detect_installer_device()  # Find USB device we booted from
detect_target_device()     # Find device to install to
show_warning_countdown()   # 15-second countdown (returns: auto/interactive)
interactive_device_menu()  # Show device selection menu
create_partition_table()   # Parse YAML, create GPT (in order)
write_partitions()         # dd images with progress
expand_last_partition()    # Grow to fill disk
install_bootloader()       # Setup systemd-boot/GRUB
cleanup_and_exit()         # Final actions

# Main workflow
main() {
    clear
    show_banner
    TARGET_DEVICE=$(detect_target_device)

    # Show countdown - returns mode (auto/interactive)
    MODE=$(show_warning_countdown "$TARGET_DEVICE")

    if [[ "$MODE" == "interactive" ]]; then
        # User pressed key - show device menu
        TARGET_DEVICE=$(interactive_device_menu)
        [[ -z "$TARGET_DEVICE" ]] && exit 0  # User chose abort
    fi

    create_partition_table "$TARGET_DEVICE"
    write_partitions "$TARGET_DEVICE"
    expand_last_partition "$TARGET_DEVICE"
    install_bootloader "$TARGET_DEVICE"
    show_success
    poweroff
}
```

---

### 3. Device Detection Logic

**Challenge**: Distinguish between installer USB and target device

**Strategy**:
1. Find device containing `/images` directory (installer device)
2. List all block devices excluding:
   - The installer device
   - Loop devices
   - RAM disks
   - Mounted removable media
3. If one device remains → use it
4. If multiple devices → prompt user
5. If no devices → error and halt

**Implementation**:
```bash
detect_installer_device() {
    # Find device mounted at /images or containing running root
    local root_dev=$(findmnt -n -o SOURCE /)
    echo "$root_dev"
}

detect_target_device() {
    local installer_dev=$(detect_installer_device)
    local devices=$(lsblk -dpno NAME,TYPE,SIZE,TRAN | grep disk | grep -v "$installer_dev")

    # Filter logic here
    # Return single device or prompt
}
```

---

### 4. Warning & Countdown System

**Requirements**:
- Clear visual warning
- Display target device info (name, size)
- 15-second countdown
- Any keypress enters interactive mode
- Visual progress bar

**Behavior**:
- **No keypress**: Automatically proceed with detected device
- **Keypress detected**: Enter interactive menu to select device or abort

**User Interface**:
```
╔═══════════════════════════════════════════════════════════╗
║  TelemetryTV Star Forge Installer                     ║
╠═══════════════════════════════════════════════════════════╣
║                                                           ║
║  Target Device: /dev/nvme0n1                             ║
║  Size: 512 GB                                            ║
║  Distribution: development (27GB)                         ║
║                                                           ║
║  ⚠️  WARNING: ALL DATA ON THIS DEVICE WILL BE ERASED!    ║
║                                                           ║
║  Press ANY KEY for device selection menu...              ║
║                                                           ║
║  [████████████████░░░░░░] 9 seconds remaining            ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
```

**Interactive Device Menu** (shown if user presses key):
```
╔═══════════════════════════════════════════════════════════╗
║  Device Selection Menu                                    ║
╠═══════════════════════════════════════════════════════════╣
║                                                           ║
║  Select target device for installation:                   ║
║                                                           ║
║  1) /dev/nvme0n1  (512 GB)  [Samsung SSD 970 EVO]        ║
║  2) /dev/sda      (1 TB)    [WDC WD10EZEX-08]            ║
║  3) /dev/sdb      (256 GB)  [USB Drive - EXCLUDED]       ║
║                                                           ║
║  A) Abort installation                                    ║
║                                                           ║
║  Enter selection [1-3, A]:                                ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
```

**Implementation Approach**:
```bash
show_warning_countdown() {
    local device=$1
    local countdown=15

    # Set terminal to raw mode to catch keypresses
    stty -echo -icanon time 0 min 0

    while [ $countdown -gt 0 ]; do
        draw_warning_screen "$device" $countdown
        sleep 1

        # Check for keypress
        if read -t 0 key; then
            stty sane
            echo "interactive"  # Return interactive mode
            return
        fi

        ((countdown--))
    done

    stty sane
    echo "auto"  # Return auto mode
}

interactive_device_menu() {
    # List all available devices (excluding installer device)
    local devices=($(lsblk -dpno NAME,SIZE,MODEL | grep -v "$(detect_installer_device)"))

    # Display menu
    clear
    draw_device_menu "${devices[@]}"

    # Read user choice
    read -p "Enter selection: " choice

    case "$choice" in
        [1-9]) echo "${devices[$((choice-1))]}" ;;
        [Aa]) echo "" ;;  # Empty = abort
        *) interactive_device_menu ;;  # Invalid, show again
    esac
}
```

---

### 5. Partition Table Creation

**Source**: Parse `/images/partitions.yaml`

**Process**:
1. Read YAML using `yq`
2. Calculate partition sizes and offsets
3. Create GPT partition table with `parted`
4. **Create partitions in exact order from YAML** (do not reorder)
5. Set partition types (EFI, Linux filesystem)
6. Set partition labels

**Important**: Partitions must be created in the exact order they appear in `partitions.yaml`. This ensures partition numbering matches expectations (e.g., partition 1 is always boot, partition 4 is always root, etc.).

**Partition Mapping**:
- `type: efi` → EFI System Partition (ESP)
- `type: linux` → Linux filesystem
- `filesystem: vfat` → FAT32
- `filesystem: ext4` → ext4

**Example Commands**:
```bash
parted -s "$device" mklabel gpt

# For each partition in YAML:
parted -s "$device" mkpart "$name" "$filesystem" "${start}MiB" "${end}MiB"

# Set EFI flag for boot partition:
parted -s "$device" set 1 esp on
```

---

### 6. Partition Image Writing

**Requirements**:
- Write images to partitions sequentially **in YAML order**
- Show progress for each partition
- Verify writes (optional checksums)
- Handle errors gracefully

**Important**: Images are written to partitions in the order they appear in `partitions.yaml`. Partition 1 gets the first image, partition 2 gets the second image, etc.

**Implementation**:
```bash
write_partitions() {
    local device=$1
    local partition_num=1

    # Parse partitions.yaml in order
    local partition_count=$(yq '.partitions | length' "$PARTITIONS_YAML")

    for ((i=0; i<partition_count; i++)); do
        local image=$(yq ".partitions[$i].image" "$PARTITIONS_YAML")
        local name=$(yq ".partitions[$i].name" "$PARTITIONS_YAML")

        log_info "Writing $name partition ($image) to ${device}${partition_num}..."

        # Detect optimal block size
        local bs=$(blockdev --getpbsz "${device}${partition_num}")

        # Write with progress
        pv "$IMAGES_DIR/$image" | \
            dd of="${device}${partition_num}" bs="$bs" oflag=direct status=none

        sync
        ((partition_num++))
    done
}
```

---

### 7. Filesystem Expansion

**Purpose**: Grow the last partition (typically `data`) to fill remaining disk space

**Process**:
1. Identify last partition
2. Expand partition to end of disk with `parted`
3. Inform kernel of changes with `partprobe`
4. Resize filesystem (`resize2fs` for ext4, `fatresize` for vfat)

**Implementation**:
```bash
expand_last_partition() {
    local device=$1
    local last_part_num=$(parted -s "$device" print | tail -n 2 | head -n 1 | awk '{print $1}')
    local last_part="${device}${last_part_num}"

    log_info "Expanding partition $last_part_num to fill disk..."

    # Expand partition
    parted -s "$device" resizepart "$last_part_num" 100%
    partprobe "$device"

    # Expand filesystem (detect type first)
    local fstype=$(lsblk -no FSTYPE "$last_part")

    case "$fstype" in
        ext4)
            e2fsck -fy "$last_part"
            resize2fs "$last_part"
            ;;
        vfat)
            fatresize -s max "$last_part"
            ;;
    esac
}
```

---

### 8. Bootloader Installation

**Decision Point**: systemd-boot (UEFI-only) vs GRUB (broader compatibility)

**Recommendation**: systemd-boot (simpler, modern UEFI systems)

**Process**:
1. Mount target partitions in temporary location
2. Mount special filesystems (proc, sys, dev)
3. Chroot into target
4. Install bootloader to EFI partition
5. Configure boot entries
6. Unmount and cleanup

**Implementation**:
```bash
install_bootloader() {
    local device=$1
    local temp_mount=$(mktemp -d)

    # Mount root partition
    mount "${device}2" "$temp_mount"  # Assuming partition 2 is root
    mount "${device}1" "$temp_mount/boot"  # Partition 1 is EFI

    # Mount special filesystems
    mount -t proc proc "$temp_mount/proc"
    mount -t sysfs sys "$temp_mount/sys"
    mount --rbind /dev "$temp_mount/dev"

    # Install systemd-boot
    arch-chroot "$temp_mount" bootctl install

    # Create boot entry
    cat > "$temp_mount/boot/loader/entries/telemetrytv.conf" <<EOF
title   TelemetryTV Star Forge
linux   /vmlinuz-linux
initrd  /initramfs-linux.img
options root=${device}2 rw
EOF

    # Cleanup
    umount -R "$temp_mount"
    rmdir "$temp_mount"
}
```

---

### 9. Shell Profile Auto-start

**Location**: `/home/installer/.bash_profile`

**Purpose**: Launch installer script only on tty1 (console)

**Configuration**:
```bash
#!/bin/bash

# Only run installer on first virtual terminal
if [[ "$(tty)" == "/dev/tty1" ]]; then
    # Execute installer script
    exec /usr/local/bin/auto-installer.sh
fi

# If on other ttys, show normal shell (for debugging)
```

**Why `exec`?**
- Replaces shell process with installer script
- Prevents user from returning to shell
- Cleaner process tree

---

### 10. Completion Handling

**Success Path**:
```
╔═══════════════════════════════════════════════╗
║  ✓ Installation Complete!                    ║
╠═══════════════════════════════════════════════╣
║                                               ║
║  Distribution installed successfully to:      ║
║  /dev/nvme0n1                                ║
║                                               ║
║  Partitions created:                          ║
║    • boot (1GB, EFI)                         ║
║    • fallback-recovery (4GB)                  ║
║    • recovery (4GB)                           ║
║    • root (4GB)                               ║
║    • var_log (1GB)                            ║
║    • data (476GB, expanded)                   ║
║                                               ║
║  System will power off in 5 seconds...       ║
║                                               ║
╚═══════════════════════════════════════════════╝
```

**Failure Path**:
```
╔═══════════════════════════════════════════════╗
║  ✗ Installation Failed                        ║
╠═══════════════════════════════════════════════╣
║                                               ║
║  Error: Failed to write root partition        ║
║                                               ║
║  Installation log saved to:                   ║
║  /var/log/installer.log                       ║
║                                               ║
║  Press any key to view log...                 ║
║  (or wait 30s for automatic shell)            ║
║                                               ║
╚═══════════════════════════════════════════════╝
```

**Actions**:
- Success: `poweroff` after 5-second delay
- Failure: Show error, offer log view, drop to shell

---

## Safety Features

### Device Protection
- Never install to device with mounted partitions (except installer)
- Exclude USB devices by default (with override option)
- Require explicit confirmation for devices < 64GB or > 2TB
- Sanity check: Verify `/images` exists before proceeding

### Error Handling
- `set -e` for immediate exit on errors
- Trap EXIT for cleanup (unmount, remove temp files)
- Detailed logging to `/var/log/installer.log`
- Preserve logs even after errors

### Validation Checks
- Verify `partitions.yaml` syntax before starting
- Check image file existence and sizes
- Verify target device has sufficient space
- Validate partition table after creation
- Checksum verification (optional, time permitting)

---

## Implementation Checklist

- [ ] 1. Create installer user and configure auto-login on tty1
- [ ] 2. Create main installation script (`/usr/local/bin/auto-installer.sh`)
- [ ] 3. Implement target device detection logic
- [ ] 4. Implement 15-second countdown with interactive mode trigger
- [ ] 5. Implement interactive device selection menu (triggered by keypress)
- [ ] 6. Implement partition table creation from `partitions.yaml` (in exact order)
- [ ] 7. Implement partition image writing with progress display (in exact order)
- [ ] 8. Implement filesystem expansion for last partition
- [ ] 9. Implement bootloader installation (systemd-boot or GRUB)
- [ ] 10. Configure installer user's shell profile to auto-start installer
- [ ] 11. Add success/failure handling and system shutdown/reboot

---

## Open Questions

### 1. Bootloader Choice
**Options**:
- **systemd-boot**: Simpler, UEFI-only, native to Arch
- **GRUB**: More compatible, works with BIOS and UEFI

**Recommendation**: systemd-boot (target hardware is modern UEFI)

### 2. Target Device Detection
**Decision**: ✅ **DECIDED**
- **Automatic with interactive fallback**:
  - Auto-detect and show in countdown
  - If user presses any key → show device selection menu
  - User can choose different device or abort
  - If no keypress in 15 seconds → proceed with auto-detected device

### 3. Post-Install Action
**Options**:
- **Power off**: Safe default, remove USB manually
- **Reboot**: Boot into new system immediately
- **Prompt user**: Ask what to do

**Recommendation**: Power off (prevents accidental re-install)

### 4. Network Access
**Question**: Does installer need network during install?

**Consideration**:
- Not required for basic installation
- Could enable for post-install updates
- Adds complexity (DHCP, DNS, etc.)

**Recommendation**: No network requirement for v1

### 5. Error Recovery
**Options**:
- **Drop to shell**: Allow manual debugging
- **Show error and halt**: Display error, wait for power off
- **Retry**: Attempt installation again

**Recommendation**: Drop to shell with clear error message

---

## Testing Strategy

### Test Environments
1. **Virtual Machine** (QEMU/VirtualBox)
   - Test full installation flow
   - Verify boot sequence
   - Test error handling

2. **Target Hardware** (if available)
   - Validate device detection
   - Test actual performance
   - Verify bootloader on real UEFI

### Test Scenarios
- [ ] Normal installation flow (happy path)
- [ ] Abort during countdown
- [ ] Multiple target devices (selection logic)
- [ ] Insufficient disk space
- [ ] Missing image files
- [ ] Corrupted `partitions.yaml`
- [ ] Bootloader installation failure
- [ ] Power loss simulation (idempotency)

---

## Future Enhancements

### Phase 2 Features
- **Interactive mode**: Menu-driven interface with device selection
- **Partition customization**: Allow user to modify sizes
- **Multiple distributions**: Choose which distro to install
- **Network installation**: Download images on-the-fly
- **Progress persistence**: Resume interrupted installations
- **SSH access**: Remote installation monitoring
- **Hardware detection**: Auto-configure based on detected hardware
- **Post-install configuration**: Network, users, hostname setup

### Advanced Features
- **Verification**: SHA256 checksums for all images
- **Encryption**: LUKS support for data partition
- **RAID**: Multi-disk installation support
- **Logging server**: Send logs to remote syslog
- **Web UI**: Browser-based installation interface

---

## File Structure Summary

```
mnt/  (development-installer mounted here)
├── etc/
│   └── systemd/
│       └── system/
│           └── getty@tty1.service.d/
│               └── autologin.conf          # Auto-login config
├── home/
│   └── installer/
│       └── .bash_profile                   # Auto-start script
├── usr/
│   └── local/
│       └── bin/
│           └── auto-installer.sh           # Main installer script
├── images/                                 # Already exists
│   ├── partitions.yaml                     # Already exists
│   ├── boot.img                            # Already exists
│   ├── root.img                            # Already exists
│   ├── recovery.img                        # Already exists
│   ├── recovery_factory.img                # Already exists
│   ├── var_log.img                         # Already exists
│   └── data.img                            # Already exists
└── var/
    └── log/
        └── installer.log                   # Created during install
```

---

## Timeline Estimate

**Total**: ~4-6 hours for full implementation and testing

1. User setup & auto-login: 30 minutes
2. Main installer script skeleton: 1 hour
3. Device detection: 45 minutes
4. Countdown & UI: 45 minutes
5. Partition creation: 1 hour
6. Image writing: 45 minutes
7. Bootloader installation: 1 hour
8. Testing & debugging: 1-2 hours

---

## Notes

- All operations must be performed in a chroot or while mounted at `./mnt/`
- Use existing `scripts/common.sh` patterns for consistency
- Leverage existing tools (`yq`, `pv`, `parted`) already in project
- Follow project's bash style and error handling conventions
- Document all assumptions and decisions inline

---

**Created**: 2025-10-29
**Author**: Claude (AI Assistant)
**Status**: Planning Phase
**Next Step**: Begin implementation with user/auto-login setup
