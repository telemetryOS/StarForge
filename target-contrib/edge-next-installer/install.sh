#!/bin/bash
# TelemetryOS Edge Installer
# Automatically installs TelemetryOS Edge distribution images to a target disk
#
# When /images/recovery-mode.yaml exists, runs in recovery mode:
#   - Auto-detects own disk from /proc/cmdline
#   - Skips partitioning — flashes images to existing partitions by UUID
#   - Resets boot counting for systemd-boot blessed boot
#   - Tracks recovery attempts on data partition
#   - Reboots automatically on completion

# Don't use set -e - we want to handle errors explicitly
set +e

# Configuration
IMAGES_DIR="/images"
PARTITIONS_CONFIG="$IMAGES_DIR/partitions.yaml"
RECOVERY_CONFIG="$IMAGES_DIR/recovery-mode.yaml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1;37m'
NC='\033[0m'

# Logging functions
log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

print_header() {
    local text="$1"
    echo ""
    echo -e "${BLUE}─────────────────────────────────────${NC}"
    echo -e "          ${BOLD}$text${NC}"
    echo -e "${BLUE}─────────────────────────────────────${NC}"
    echo ""
}

log_step() {
    local step="$1"
    local total="$2"
    local text="$3"
    echo ""
    echo -e "${BLUE}[Step $step/$total]${NC} ${BOLD}$text${NC}"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This installer must be run as root"
        exit 1
    fi
}

# Check required commands
check_requirements() {
    local missing=()

    for cmd in yq blkid mkfs.ext4 mkfs.vfat parted bootctl efibootmgr resize2fs e2fsck zstd pv genfstab; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required commands: ${missing[*]}"
        exit 1
    fi
}

# Check if partitions config exists
check_config() {
    if [[ ! -f "$PARTITIONS_CONFIG" ]]; then
        log_error "Partition configuration not found: $PARTITIONS_CONFIG"
        log_info "Load distribution images first with: sf load-installer"
        exit 1
    fi
}

# Get installer's own disk (to exclude from target list)
get_installer_disk() {
    local images_mount=$(findmnt -n -o SOURCE /images 2>/dev/null)
    if [[ -z "$images_mount" ]]; then
        echo ""
        return
    fi

    local disk=$(lsblk -n -d -o PKNAME "$images_mount" 2>/dev/null)
    echo "/dev/$disk"
}

# Get list of available disks
get_available_disks() {
    local installer_disk=$(get_installer_disk)
    local -a disks=()

    # List all block devices
    for disk in /dev/sd? /dev/sd?? /dev/vd? /dev/vd?? /dev/nvme?n? /dev/nvme??n? /dev/mmcblk?; do
        # Skip if not a block device
        [[ ! -b "$disk" ]] && continue

        # Skip installer disk
        [[ -n "$installer_disk" && "$disk" == "$installer_disk" ]] && continue

        # Make sure it's actually a disk
        local type=$(lsblk -n -d -o TYPE "$disk" 2>/dev/null)
        [[ "$type" != "disk" ]] && continue

        disks+=("$disk")
    done

    echo "${disks[@]}"
}

# Check for interactive mode with timeout
check_interactive_mode() {
    echo ""
    echo "Press 's' to drop to shell"
    echo "Press any other key for interactive installation"
    echo "Or wait for automatic installation to first available disk"
    echo ""

    # Countdown with live updates
    local seconds=15
    local key=""

    while [[ $seconds -gt 0 ]]; do
        echo -ne "\r${YELLOW}Starting in ${seconds} seconds...${NC}  "

        # Read with 1 second timeout
        if read -t 1 -n 1 -s key; then
            echo ""
            echo ""

            if [[ "$key" == "s" || "$key" == "S" ]]; then
                log_info "Dropping to shell..."
                echo ""
                echo "─────────────────────────────────────"
                echo "Debug Shell"
                echo "─────────────────────────────────────"
                echo ""
                echo "Type 'exit' to return to installer menu"
                echo ""

                # Start an interactive shell and wait for it to complete
                PS1="[debug] # " bash --norc

                # Clear screen and restart the menu after shell exits
                clear
                check_interactive_mode
                return $?
            else
                log_info "Interactive mode selected"
                echo ""
                return 0  # Interactive
            fi
        fi

        ((seconds--))
    done

    echo ""
    echo ""
    log_info "Starting automatic installation..."
    echo ""
    return 1  # Automatic
}

# Show available disks and let user select
select_target_disk() {
    local auto_mode="$1"
    local disks=($(get_available_disks))

    if [[ ${#disks[@]} -eq 0 ]]; then
        echo ""
        log_error "No suitable disks found for installation"
        echo ""
        echo "This installer needs a target disk to install TelemetryOS Edge to."
        echo ""
        echo "Possible reasons:"
        echo "  - No additional disk is connected"
        echo "  - Target disk is too small"
        echo "  - All available disks are in use"
        echo ""
        log_info "Connect a target disk and reboot to try again"
        echo ""
        read -p "Press Enter to reboot..."
        reboot
    fi

    # Automatic mode - select first disk
    if [[ "$auto_mode" == "true" ]]; then
        local selected_disk="${disks[0]}"
        local size=$(lsblk -n -d -o SIZE "$selected_disk" 2>/dev/null || echo "Unknown")
        local model=$(lsblk -n -d -o MODEL "$selected_disk" 2>/dev/null | xargs || echo "Unknown")
        [[ -z "$model" || "$model" == "" ]] && model="Unknown"

        log_info "Automatically selected: $selected_disk" >&2
        log_info "  Model: $model" >&2
        log_info "  Size: $size" >&2
        echo "" >&2
        log_warn "Installation will begin in 5 seconds..." >&2
        sleep 5

        echo "$selected_disk"
        return 0
    fi

    # Interactive mode - show menu
    echo "Available disks for installation:" >&2
    echo "" >&2

    local i=1
    for disk in "${disks[@]}"; do
        # Get disk info, with fallbacks
        local size=$(lsblk -n -d -o SIZE "$disk" 2>/dev/null || echo "Unknown")
        local model=$(lsblk -n -d -o MODEL "$disk" 2>/dev/null | xargs || echo "Unknown")
        [[ -z "$model" || "$model" == "" ]] && model="Unknown"
        [[ -z "$size" || "$size" == "" ]] && size="Unknown"

        echo "  $i) $disk - $model ($size)" >&2
        ((i++))
    done

    echo "" >&2
    echo -e "${RED}WARNING: ALL DATA ON THE SELECTED DISK WILL BE DESTROYED!${NC}" >&2
    echo "" >&2

    while true; do
        read -p "Select disk number (or 'q' to reboot): " choice

        if [[ "$choice" == "q" ]]; then
            log_info "Installation cancelled, rebooting..."
            sleep 2
            reboot
        fi

        if [[ "$choice" =~ ^[0-9]+$ ]] && [[ $choice -ge 1 ]] && [[ $choice -le ${#disks[@]} ]]; then
            local selected_disk="${disks[$((choice-1))]}"
            echo "" >&2
            echo "Selected: $selected_disk" >&2

            local size=$(lsblk -n -d -o SIZE "$selected_disk" 2>/dev/null)
            local model=$(lsblk -n -d -o MODEL "$selected_disk" 2>/dev/null | xargs)
            echo "  Model: $model" >&2
            echo "  Size: $size" >&2
            echo "" >&2

            read -p "Confirm installation to $selected_disk? (y/n): " confirm
            if [[ "$confirm" == "y" || "$confirm" == "Y" ]]; then
                echo "$selected_disk"
                return 0
            else
                echo "" >&2
                echo "Installation cancelled, returning to disk selection..." >&2
                echo "" >&2
            fi
        else
            echo "" >&2
            log_error "Invalid selection"
            echo "" >&2
        fi
    done
}

# Wipe disk and create GPT partition table
initialize_disk() {
    local disk="$1"

    log_info "Creating GPT partition table..."
    if ! parted -s "$disk" mklabel gpt 2>&1; then
        log_error "Failed to create GPT partition table"
        return 1
    fi

    # Inform kernel of changes
    sync
    partprobe "$disk" 2>&1 || true
    sleep 1

    return 0
}

# Create partitions on target disk
create_partitions() {
    local disk="$1"
    local partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")

    local start="1MiB"

    for i in $(seq 0 $((partition_count - 1))); do
        local name=$(yq -r ".partitions[$i].name" "$PARTITIONS_CONFIG")
        local part_type=$(yq -r ".partitions[$i].type" "$PARTITIONS_CONFIG")
        local size_mb=$(yq -r ".partitions[$i].size_mb" "$PARTITIONS_CONFIG")

        local end
        if [[ $i -eq $((partition_count - 1)) ]]; then
            end="100%"
        else
            end="$((${start%MiB} + size_mb))MiB"
        fi

        log_info "Creating partition $((i + 1)): $name ($size_mb MB)"

        if [[ "$part_type" == "efi" ]]; then
            parted -s "$disk" mkpart "$name" fat32 "$start" "$end" 2>&1
            parted -s "$disk" set $((i + 1)) esp on 2>&1
        else
            parted -s "$disk" mkpart "$name" ext4 "$start" "$end" 2>&1
        fi

        start="$end"
    done

    partprobe "$disk" 2>&1
    sleep 2
}

# Write images to partitions (install mode — sequential partition numbering)
write_images() {
    local disk="$1"
    local partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")

    for i in $(seq 0 $((partition_count - 1))); do
        local name=$(yq -r ".partitions[$i].name" "$PARTITIONS_CONFIG")
        local image=$(yq -r ".partitions[$i].image" "$PARTITIONS_CONFIG")
        local filesystem=$(yq -r ".partitions[$i].filesystem" "$PARTITIONS_CONFIG")

        local image_path="$IMAGES_DIR/$image"
        local partition="${disk}$((i + 1))"

        # Handle nvme and mmcblk devices (use p1, p2 naming)
        if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
            partition="${disk}p$((i + 1))"
        fi

        # Skip partitions marked as "empty" — create filesystem only
        if [[ "$image" == "empty" ]]; then
            echo -e "${CYAN}[$((i + 1))/$partition_count]${NC} ${BOLD}$name${NC} → $partition (creating empty filesystem)"
            if [[ "$filesystem" == "ext4" ]]; then
                mkfs.ext4 -qF "$partition" 2>&1
            elif [[ "$filesystem" == "vfat" ]]; then
                mkfs.vfat -F 32 "$partition" 2>&1
            fi

            echo ""
            continue
        fi

        if [[ ! -f "$image_path" ]]; then
            log_error "Image not found: $image_path"
            return 1
        fi

        # Get compressed/uncompressed size for progress bar
        local source_size
        if [[ "$image_path" == *.zst ]]; then
            source_size=$(stat -c%s "$image_path")
        else
            source_size=$(stat -c%s "$image_path")
        fi

        # Display progress header
        echo -e "${CYAN}[$((i + 1))/$partition_count]${NC} ${BOLD}$name${NC} → $partition"

        # Check if image is compressed
        if [[ "$image_path" == *.zst ]]; then
            # Decompress and write with progress bar
            pv -p -t -e -r -b -s "$source_size" "$image_path" | \
                zstd -d -c -T0 | \
                dd of="$partition" bs=32M oflag=direct conv=fsync status=none 2>&1

            # Check if any command in pipeline failed
            if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
                echo ""
                log_error "Failed to read compressed image: $image_path"
                return 1
            elif [[ ${PIPESTATUS[1]} -ne 0 ]]; then
                echo ""
                log_error "Failed to decompress $name"
                return 1
            elif [[ ${PIPESTATUS[2]} -ne 0 ]]; then
                echo ""
                log_error "Failed to write $name to $partition"
                return 1
            fi
        else
            # Write directly with progress bar
            pv -p -t -e -r -b "$image_path" | \
                dd of="$partition" bs=32M oflag=direct conv=fsync status=none 2>&1

            # Check if any command in pipeline failed
            if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
                echo ""
                log_error "Failed to read image: $image_path"
                return 1
            elif [[ ${PIPESTATUS[1]} -ne 0 ]]; then
                echo ""
                log_error "Failed to write $name to $partition"
                return 1
            fi
        fi
        echo ""
    done

    log_info "Syncing writes to disk..."
    sync
}

# Rebuild initramfs for target hardware
rebuild_initramfs() {
    local disk="$1"

    local boot_partition="${disk}1"
    local root_partition="${disk}2"

    if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
        boot_partition="${disk}p1"
        root_partition="${disk}p2"
    fi

    local temp_mount=$(mktemp -d)

    # Mount root and boot
    if ! mount "$root_partition" "$temp_mount" 2>&1; then
        log_error "Failed to mount root partition"
        rmdir "$temp_mount"
        return 1
    fi

    mkdir -p "$temp_mount/boot"
    if ! mount "$boot_partition" "$temp_mount/boot" 2>&1; then
        log_error "Failed to mount boot partition"
        umount "$temp_mount" 2>&1
        rmdir "$temp_mount"
        return 1
    fi

    # Rebuild initramfs with arch-chroot
    log_info "Detecting hardware and rebuilding initramfs..."
    if ! arch-chroot "$temp_mount" mkinitcpio -P 2>&1 | grep -v "^==>"; then
        log_warn "Failed to rebuild initramfs (system may not boot on this hardware)"
    else
        log_info "Initramfs rebuilt successfully"
    fi

    # Cleanup
    umount "$temp_mount/boot" 2>&1
    umount "$temp_mount" 2>&1
    rmdir "$temp_mount"

    return 0
}

# Expand last partition to fill disk
expand_last_partition() {
    local disk="$1"
    local partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")

    # Get the last partition number and device
    local last_part_num=$partition_count
    local last_partition="${disk}${last_part_num}"

    # Handle nvme and mmcblk devices (use p1, p2 naming)
    if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
        last_partition="${disk}p${last_part_num}"
    fi

    # Use parted to resize the partition to 100%
    log_info "Resizing partition to fill disk..."
    if ! parted -s "$disk" resizepart "$last_part_num" 100% 2>&1; then
        log_warn "Failed to resize partition (may already be at max size)"
        return 0  # Don't fail installation if resize fails
    fi

    # Inform kernel of partition changes
    partprobe "$disk" 2>&1
    sleep 1

    # Check filesystem before resizing (required by resize2fs)
    log_info "Checking filesystem on $last_partition..."
    if ! e2fsck -f -p "$last_partition" 2>&1; then
        log_warn "Filesystem check failed, attempting resize anyway..."
    fi

    # Expand the filesystem to fill the partition
    log_info "Expanding filesystem on $last_partition..."
    if ! resize2fs "$last_partition" 2>&1; then
        log_warn "Failed to resize filesystem"
        return 0  # Don't fail installation if resize fails
    fi

    return 0
}

# Install bootloader
install_bootloader() {
    local disk="$1"

    local boot_partition="${disk}1"

    if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
        boot_partition="${disk}p1"
    fi

    log_info "Creating EFI boot entry for systemd-boot..."

    # Bootloader files are already in place from dd'd images
    # Just create a minimal EFI NVRAM entry to make it bootable

    if command -v efibootmgr &> /dev/null; then
        # Get partition number (strip device prefix, keep just the number)
        local part_num="${boot_partition##*[a-z]}"
        part_num="${part_num##*p}"  # Handle nvme/mmcblk pN naming

        # Get disk device (without partition number)
        local disk_dev="$disk"

        # Create one EFI entry pointing to systemd-boot
        if efibootmgr --create --disk "$disk_dev" --part "$part_num" \
            --label "TelemetryOS Edge" \
            --loader '\EFI\systemd\systemd-bootx64.efi' >/dev/null 2>&1; then
            log_info "Created EFI boot entry"
        else
            log_warn "Failed to create EFI entry (system may still boot via EFI fallback)"
        fi
    else
        log_warn "efibootmgr not available"
        log_info "System should still boot via EFI fallback mechanism"
    fi

    return 0
}

# Regenerate fstab and update boot entries with correct UUIDs from target disk
configure_uuids() {
    local disk="$1"
    local partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")
    local temp_root=$(mktemp -d)

    # Mount root partition (mount_point ".") first
    for i in $(seq 0 $((partition_count - 1))); do
        local mp=$(yq -r ".partitions[$i].mount_point" "$PARTITIONS_CONFIG")
        if [[ "$mp" == "." ]]; then
            local part_num=$((i + 1))
            local dev="${disk}${part_num}"
            [[ "$disk" == *"nvme"* || "$disk" == *"mmcblk"* ]] && dev="${disk}p${part_num}"

            if ! mount "$dev" "$temp_root" 2>&1; then
                log_error "Failed to mount root partition"
                rmdir "$temp_root"
                return 1
            fi
            break
        fi
    done

    # Mount remaining partitions
    for i in $(seq 0 $((partition_count - 1))); do
        local mp=$(yq -r ".partitions[$i].mount_point" "$PARTITIONS_CONFIG")
        [[ "$mp" == "." ]] && continue

        local part_num=$((i + 1))
        local dev="${disk}${part_num}"
        [[ "$disk" == *"nvme"* || "$disk" == *"mmcblk"* ]] && dev="${disk}p${part_num}"

        mkdir -p "$temp_root/$mp"
        if ! mount "$dev" "$temp_root/$mp" 2>&1; then
            log_warn "Failed to mount $dev at /$mp"
        fi
    done

    # Regenerate fstab with correct UUIDs
    log_info "Regenerating /etc/fstab..."
    genfstab -U "$temp_root" | grep -v "swap" > "$temp_root/etc/fstab"

    # Update boot entries with correct partition UUIDs
    local entries_dir="$temp_root/boot/loader/entries"
    if [[ -d "$entries_dir" ]]; then
        for i in $(seq 0 $((partition_count - 1))); do
            local name=$(yq -r ".partitions[$i].name" "$PARTITIONS_CONFIG")
            local mp=$(yq -r ".partitions[$i].mount_point" "$PARTITIONS_CONFIG")
            local part_num=$((i + 1))
            local dev="${disk}${part_num}"
            [[ "$disk" == *"nvme"* || "$disk" == *"mmcblk"* ]] && dev="${disk}p${part_num}"
            local uuid=$(blkid -s UUID -o value "$dev")

            # Map partitions to boot entries:
            # root (mount_point ".") → arch.conf
            # named partitions → <name>.conf (if entry exists)
            local entry_file=""
            if [[ "$mp" == "." ]]; then
                entry_file="$entries_dir/arch.conf"
            elif [[ -f "$entries_dir/$name.conf" ]]; then
                entry_file="$entries_dir/$name.conf"
            fi

            if [[ -n "$entry_file" ]]; then
                log_info "Updating $(basename "$entry_file") (UUID: $uuid)"
                sed -i "s|root=UUID=[^ ]*|root=UUID=$uuid|" "$entry_file"
            fi
        done
    fi

    # Apply ownership and permissions from partitions.yaml
    for i in $(seq 0 $((partition_count - 1))); do
        local owner=$(yq -r ".partitions[$i].owner // \"\"" "$PARTITIONS_CONFIG")
        local group=$(yq -r ".partitions[$i].group // \"\"" "$PARTITIONS_CONFIG")
        local mode=$(yq -r ".partitions[$i].mode // \"\"" "$PARTITIONS_CONFIG")

        if [[ -n "$owner" || -n "$group" || -n "$mode" ]]; then
            local mp=$(yq -r ".partitions[$i].mount_point" "$PARTITIONS_CONFIG")
            local target_path="$temp_root/$mp"
            [[ "$mp" == "." ]] && target_path="$temp_root"

            if [[ -n "$owner" || -n "$group" ]]; then
                # Resolve names to UIDs from target system's passwd/group
                local uid="" gid=""
                if [[ -n "$owner" ]]; then
                    uid=$(grep "^${owner}:" "$temp_root/etc/passwd" 2>/dev/null | cut -d: -f3)
                    [[ -z "$uid" ]] && uid="$owner"
                fi
                if [[ -n "$group" ]]; then
                    gid=$(grep "^${group}:" "$temp_root/etc/group" 2>/dev/null | cut -d: -f3)
                    [[ -z "$gid" ]] && gid="$group"
                fi
                chown "${uid:-0}:${gid:-0}" "$target_path"
            fi

            if [[ -n "$mode" ]]; then
                chmod "$mode" "$target_path"
            fi

            log_info "Set /$mp permissions: ${owner:-root}:${group:-root} ${mode:-default}"
        fi
    done

    # Unmount all partitions
    umount -R "$temp_root" 2>&1
    rmdir "$temp_root" 2>/dev/null || true

    return 0
}

# ─────────────────────────────────────────────────────────────────────────────
# Shared Helpers
# ─────────────────────────────────────────────────────────────────────────────

# Get partition device path by name (looks up index in partitions.yaml)
get_partition_device() {
    local disk="$1"
    local part_name="$2"
    local partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")

    for i in $(seq 0 $((partition_count - 1))); do
        local name=$(yq -r ".partitions[$i].name" "$PARTITIONS_CONFIG")
        if [[ "$name" == "$part_name" ]]; then
            local part_num=$((i + 1))
            if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
                echo "${disk}p${part_num}"
            else
                echo "${disk}${part_num}"
            fi
            return 0
        fi
    done
    return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Recovery Mode Functions
# ─────────────────────────────────────────────────────────────────────────────

# Get the disk device we booted from (recovery partition's parent disk)
get_own_disk() {
    # Extract root UUID from kernel command line
    local root_uuid=""
    local cmdline
    cmdline=$(cat /proc/cmdline)

    if [[ "$cmdline" =~ root=UUID=([^ ]+) ]]; then
        root_uuid="${BASH_REMATCH[1]}"
    else
        log_error "Cannot determine root UUID from /proc/cmdline"
        return 1
    fi

    # Find the device for this UUID
    local root_dev
    root_dev=$(blkid -U "$root_uuid" 2>/dev/null)
    if [[ -z "$root_dev" ]]; then
        log_error "Cannot find device for UUID $root_uuid"
        return 1
    fi

    # Get the parent disk
    local disk
    disk=$(lsblk -n -d -o PKNAME "$root_dev" 2>/dev/null)
    if [[ -z "$disk" ]]; then
        log_error "Cannot determine parent disk for $root_dev"
        return 1
    fi

    echo "/dev/$disk"
}

# Find a partition device by its UUID
find_device_by_uuid() {
    local uuid="$1"
    local dev
    dev=$(blkid -U "$uuid" 2>/dev/null)
    if [[ -z "$dev" ]]; then
        return 1
    fi
    echo "$dev"
}

# Write a single image to a device (used by recovery mode)
write_image_to_device() {
    local name="$1"
    local image_path="$2"
    local device="$3"
    local index="$4"
    local total="$5"

    if [[ ! -f "$image_path" ]]; then
        log_error "Image not found: $image_path"
        return 1
    fi

    local source_size
    source_size=$(stat -c%s "$image_path")

    echo -e "${CYAN}[$index/$total]${NC} ${BOLD}$name${NC} → $device"

    if [[ "$image_path" == *.zst ]]; then
        pv -p -t -e -r -b -s "$source_size" "$image_path" | \
            zstd -d -c -T0 | \
            dd of="$device" bs=32M oflag=direct conv=fsync status=none 2>&1

        if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
            echo ""
            log_error "Failed to read compressed image: $image_path"
            return 1
        elif [[ ${PIPESTATUS[1]} -ne 0 ]]; then
            echo ""
            log_error "Failed to decompress $name"
            return 1
        elif [[ ${PIPESTATUS[2]} -ne 0 ]]; then
            echo ""
            log_error "Failed to write $name to $device"
            return 1
        fi
    else
        pv -p -t -e -r -b "$image_path" | \
            dd of="$device" bs=32M oflag=direct conv=fsync status=none 2>&1

        if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
            echo ""
            log_error "Failed to read image: $image_path"
            return 1
        elif [[ ${PIPESTATUS[1]} -ne 0 ]]; then
            echo ""
            log_error "Failed to write $name to $device"
            return 1
        fi
    fi
    echo ""
    return 0
}

# Write images to partitions by UUID lookup (recovery mode)
write_images_by_uuid() {
    local partition_count
    partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")

    local written=0
    for i in $(seq 0 $((partition_count - 1))); do
        local name
        name=$(yq -r ".partitions[$i].name" "$PARTITIONS_CONFIG")
        local image
        image=$(yq -r ".partitions[$i].image" "$PARTITIONS_CONFIG")

        # Look up target UUID from recovery-mode.yaml
        local target_uuid
        target_uuid=$(yq -r ".partition_uuids.$name" "$RECOVERY_CONFIG")
        if [[ -z "$target_uuid" || "$target_uuid" == "null" ]]; then
            log_warn "No UUID mapping for partition '$name' in recovery-mode.yaml, skipping"
            continue
        fi

        # Find the device by UUID
        local device
        device=$(find_device_by_uuid "$target_uuid")
        if [[ -z "$device" ]]; then
            log_error "Cannot find device for $name (UUID: $target_uuid)"
            return 1
        fi

        ((written++))
        if ! write_image_to_device "$name" "$IMAGES_DIR/$image" "$device" "$written" "$partition_count"; then
            return 1
        fi
    done

    log_info "Syncing writes to disk..."
    sync
}

# Reset boot counting — write fresh arch+N.conf and clean up old entries
reset_boot_counting() {
    local boot_uuid
    boot_uuid=$(yq -r '.boot_uuid' "$RECOVERY_CONFIG")
    local boot_entry
    boot_entry=$(yq -r '.boot_entry' "$RECOVERY_CONFIG")
    local boot_tries
    boot_tries=$(yq -r '.boot_tries' "$RECOVERY_CONFIG")

    if [[ -z "$boot_uuid" || "$boot_uuid" == "null" ]]; then
        log_error "No boot_uuid in recovery-mode.yaml"
        return 1
    fi

    local boot_dev
    boot_dev=$(find_device_by_uuid "$boot_uuid")
    if [[ -z "$boot_dev" ]]; then
        log_error "Cannot find boot partition (UUID: $boot_uuid)"
        return 1
    fi

    # Default values
    [[ -z "$boot_entry" || "$boot_entry" == "null" ]] && boot_entry="arch"
    [[ -z "$boot_tries" || "$boot_tries" == "null" ]] && boot_tries="3"

    local boot_mount
    boot_mount=$(mktemp -d)

    if ! mount "$boot_dev" "$boot_mount" 2>&1; then
        log_error "Failed to mount boot partition"
        rmdir "$boot_mount"
        return 1
    fi

    local entries_dir="$boot_mount/loader/entries"

    # Remove all arch boot entries (blessed, counting, and bad variants)
    log_info "Removing old boot entries..."
    rm -f "$entries_dir"/${boot_entry}.conf
    rm -f "$entries_dir"/${boot_entry}+*.conf

    # Read the root UUID from recovery-mode.yaml to build the new entry
    local root_uuid
    root_uuid=$(yq -r '.partition_uuids.root' "$RECOVERY_CONFIG")
    if [[ -z "$root_uuid" || "$root_uuid" == "null" ]]; then
        log_error "No root UUID in recovery-mode.yaml partition_uuids"
        umount "$boot_mount" 2>&1
        rmdir "$boot_mount"
        return 1
    fi

    # Write fresh boot counting entry: arch+3.conf
    local new_entry="$entries_dir/${boot_entry}+${boot_tries}.conf"
    log_info "Writing $new_entry..."
    cat > "$new_entry" << EOF
title   TelemetryOS Edge
sort-key tos-0
linux   /vmlinuz-linux
initrd  /initramfs-linux.img
options root=UUID=${root_uuid} rw quiet splash fsck.mode=force fsck.repair=yes audit=0
EOF

    umount "$boot_mount" 2>&1
    rmdir "$boot_mount"

    log_info "Boot counting reset (${boot_entry}+${boot_tries})"
    return 0
}

# Redirect boot to fallback-recovery (removes recovery.conf from boot partition)
redirect_to_fallback_recovery() {
    local boot_uuid
    boot_uuid=$(yq -r '.boot_uuid' "$RECOVERY_CONFIG")

    if [[ -z "$boot_uuid" || "$boot_uuid" == "null" ]]; then
        log_error "No boot_uuid in recovery-mode.yaml"
        return 1
    fi

    local boot_dev
    boot_dev=$(find_device_by_uuid "$boot_uuid")
    if [[ -z "$boot_dev" ]]; then
        log_error "Cannot find boot partition (UUID: $boot_uuid)"
        return 1
    fi

    local boot_mount
    boot_mount=$(mktemp -d)

    log_info "Redirecting boot to fallback-recovery..."
    if ! mount "$boot_dev" "$boot_mount" 2>&1; then
        log_error "Failed to mount boot partition"
        rmdir "$boot_mount"
        return 1
    fi

    # Remove recovery.conf so systemd-boot skips to fallback-recovery
    rm -f "$boot_mount/loader/entries/recovery.conf"
    log_info "  Removed recovery.conf — next boot will use fallback-recovery"

    umount "$boot_mount" 2>&1
    rmdir "$boot_mount"
    return 0
}

# Check and increment recovery counter on data partition
# Returns 0 if recovery should proceed, 1 if max exceeded (should fallback)
check_recovery_counter() {
    local data_uuid
    data_uuid=$(yq -r '.data_uuid' "$RECOVERY_CONFIG")
    local max_count
    max_count=$(yq -r '.max_recovery_count' "$RECOVERY_CONFIG")

    if [[ -z "$data_uuid" || "$data_uuid" == "null" ]]; then
        log_warn "No data_uuid in recovery-mode.yaml, skipping recovery counter"
        return 0
    fi

    [[ -z "$max_count" || "$max_count" == "null" ]] && max_count="3"

    local data_dev
    data_dev=$(find_device_by_uuid "$data_uuid")
    if [[ -z "$data_dev" ]]; then
        log_warn "Cannot find data partition (UUID: $data_uuid), skipping recovery counter"
        return 0
    fi

    local data_mount
    data_mount=$(mktemp -d)

    if ! mount "$data_dev" "$data_mount" 2>&1; then
        log_warn "Cannot mount data partition, skipping recovery counter"
        rmdir "$data_mount"
        return 0
    fi

    local counter_file="$data_mount/.recovery-count"
    local current_count=0

    if [[ -f "$counter_file" ]]; then
        current_count=$(cat "$counter_file" 2>/dev/null)
        # Sanitize — ensure it's a number
        if ! [[ "$current_count" =~ ^[0-9]+$ ]]; then
            current_count=0
        fi
    fi

    local new_count=$((current_count + 1))
    log_info "Recovery attempt $new_count of $max_count"

    if [[ $new_count -gt $max_count ]]; then
        log_error "Maximum recovery count ($max_count) exceeded!"
        log_error "Redirecting to fallback-recovery..."
        umount "$data_mount" 2>&1
        rmdir "$data_mount"
        return 1
    fi

    # Write incremented counter
    echo "$new_count" > "$counter_file"

    umount "$data_mount" 2>&1
    rmdir "$data_mount"
    return 0
}

# Reset recovery counter to 0 (called after successful boot from root)
# This is a helper for external use, not called during recovery itself
reset_recovery_counter() {
    local data_uuid
    data_uuid=$(yq -r '.data_uuid' "$RECOVERY_CONFIG")

    if [[ -z "$data_uuid" || "$data_uuid" == "null" ]]; then
        return 0
    fi

    local data_dev
    data_dev=$(find_device_by_uuid "$data_uuid")
    if [[ -z "$data_dev" ]]; then
        return 0
    fi

    local data_mount
    data_mount=$(mktemp -d)

    if mount "$data_dev" "$data_mount" 2>&1; then
        echo "0" > "$data_mount/.recovery-count"
        umount "$data_mount" 2>&1
    fi
    rmdir "$data_mount" 2>/dev/null
}

# ─────────────────────────────────────────────────────────────────────────────
# Install Mode Functions
# ─────────────────────────────────────────────────────────────────────────────

# Populate recovery and fallback-recovery partitions with images and config
populate_recovery_partitions() {
    local disk="$1"

    # Get UUIDs from the freshly-written partitions
    sync
    sleep 2  # Let kernel settle
    partprobe "$disk" 2>&1 || true
    sleep 1

    local boot_dev=$(get_partition_device "$disk" "boot")
    local root_dev=$(get_partition_device "$disk" "root")
    local recovery_dev=$(get_partition_device "$disk" "recovery")
    local fallback_recovery_dev=$(get_partition_device "$disk" "fallback-recovery")
    local data_dev=$(get_partition_device "$disk" "data")

    local boot_uuid=$(blkid -s UUID -o value "$boot_dev")
    local root_uuid=$(blkid -s UUID -o value "$root_dev")
    local data_uuid=$(blkid -s UUID -o value "$data_dev")

    log_info "Detected partition UUIDs:"
    log_info "  boot: $boot_uuid"
    log_info "  root: $root_uuid"
    log_info "  data: $data_uuid"

    # Mount recovery and fallback-recovery
    local recovery_mount=$(mktemp -d)
    local fallback_recovery_mount=$(mktemp -d)

    if ! mount "$recovery_dev" "$recovery_mount" 2>&1; then
        log_error "Failed to mount recovery partition"
        rmdir "$recovery_mount" "$fallback_recovery_mount"
        return 1
    fi

    if ! mount "$fallback_recovery_dev" "$fallback_recovery_mount" 2>&1; then
        log_error "Failed to mount fallback-recovery partition"
        umount "$recovery_mount" 2>&1
        rmdir "$recovery_mount" "$fallback_recovery_mount"
        return 1
    fi

    # Create /images/ directories
    mkdir -p "$recovery_mount/images"
    mkdir -p "$fallback_recovery_mount/images"

    # Copy compressed images from installer's /images/
    log_info "Copying boot.img.zst to recovery partitions..."
    cp "$IMAGES_DIR/boot.img.zst" "$recovery_mount/images/boot.img.zst"
    cp "$IMAGES_DIR/boot.img.zst" "$fallback_recovery_mount/images/boot.img.zst"

    log_info "Copying root.img.zst to recovery partitions..."
    cp "$IMAGES_DIR/root.img.zst" "$recovery_mount/images/root.img.zst"
    cp "$IMAGES_DIR/root.img.zst" "$fallback_recovery_mount/images/root.img.zst"

    # Generate partitions.yaml (boot + root)
    log_info "Creating partitions.yaml..."
    local boot_size_mb=$(yq -r '.partitions[] | select(.name == "boot") | .size_mb' "$PARTITIONS_CONFIG")
    local root_size_mb=$(yq -r '.partitions[] | select(.name == "root") | .size_mb' "$PARTITIONS_CONFIG")

    cat > "$recovery_mount/images/partitions.yaml" << EOF
# Partition configuration for recovery mode
# Boot and root are reflashed during recovery
partitions:
  - name: boot
    image: boot.img.zst
    filesystem: vfat
    mount_point: boot
    type: efi
    size_mb: $boot_size_mb
  - name: root
    image: root.img.zst
    filesystem: ext4
    mount_point: .
    type: linux
    size_mb: $root_size_mb
EOF

    cp "$recovery_mount/images/partitions.yaml" "$fallback_recovery_mount/images/partitions.yaml"

    # Generate recovery-mode.yaml for recovery (max_recovery_count=3)
    log_info "Creating recovery-mode.yaml..."
    cat > "$recovery_mount/images/recovery-mode.yaml" << EOF
boot_uuid: "$boot_uuid"
data_uuid: "$data_uuid"
boot_entry: "arch"
boot_tries: 3
max_recovery_count: 3
# Map partition names to their device UUIDs (for finding target devices)
partition_uuids:
  root: "$root_uuid"
  boot: "$boot_uuid"
  data: "$data_uuid"
EOF

    # Generate recovery-mode.yaml for fallback-recovery (max_recovery_count=10)
    cat > "$fallback_recovery_mount/images/recovery-mode.yaml" << EOF
boot_uuid: "$boot_uuid"
data_uuid: "$data_uuid"
boot_entry: "arch"
boot_tries: 3
max_recovery_count: 10
# Map partition names to their device UUIDs (for finding target devices)
partition_uuids:
  root: "$root_uuid"
  boot: "$boot_uuid"
  data: "$data_uuid"
EOF

    # Unmount
    umount "$recovery_mount" 2>&1
    umount "$fallback_recovery_mount" 2>&1
    rmdir "$recovery_mount" "$fallback_recovery_mount"

    return 0
}

# ─────────────────────────────────────────────────────────────────────────────
# Main Flows
# ─────────────────────────────────────────────────────────────────────────────

# Main installation function (fresh install to new disk)
perform_installation() {
    local disk="$1"
    local steps=8

    print_header "Installing TelemetryOS Edge"

    log_step 1 $steps "Initializing disk"
    initialize_disk "$disk" || return 1

    log_step 2 $steps "Creating partitions"
    create_partitions "$disk" || return 1

    log_step 3 $steps "Writing partition images"
    write_images "$disk" || return 1

    log_step 4 $steps "Rebuilding initramfs"
    rebuild_initramfs "$disk" || return 1

    log_step 5 $steps "Expanding data partition"
    expand_last_partition "$disk" || return 1

    log_step 6 $steps "Configuring UUIDs"
    configure_uuids "$disk" || return 1

    log_step 7 $steps "Installing bootloader"
    install_bootloader "$disk" || return 1

    log_step 8 $steps "Populating recovery partitions"
    populate_recovery_partitions "$disk" || return 1

    echo ""
    log_success "Installation complete!"
    return 0
}

# Perform recovery — flash images by UUID, reset boot counting
perform_recovery() {
    local steps=2

    print_header "Recovering TelemetryOS Edge"

    # Check recovery counter before doing anything
    if ! check_recovery_counter; then
        echo ""
        log_error "Recovery limit exceeded!"
        if redirect_to_fallback_recovery; then
            log_info "Next boot will use fallback-recovery..."
            sleep 3
            reboot
        else
            log_error "Failed to redirect to fallback-recovery"
            log_info "Dropping to emergency shell..."
            echo ""
            PS1="[emergency] # " bash --norc
            reboot
        fi
    fi

    log_step 1 $steps "Writing partition images"
    write_images_by_uuid || return 1

    log_step 2 $steps "Resetting boot count"
    reset_boot_counting || return 1

    echo ""
    log_success "Recovery complete!"
    return 0
}

# Install mode — full interactive/automatic installation to a new disk
run_install_mode() {
    clear
    print_header "TelemetryOS Edge Installer"

    check_root
    check_requirements
    check_config

    if check_interactive_mode; then
        TARGET_DISK=$(select_target_disk "false")
    else
        TARGET_DISK=$(select_target_disk "true")
    fi

    if [[ -z "$TARGET_DISK" ]]; then
        log_info "Installation cancelled"
        exit 0
    fi

    if perform_installation "$TARGET_DISK"; then
        echo ""
        log_info "Remove the USB installer — rebooting in 5 seconds..."
        sleep 5
        reboot
    else
        echo ""
        log_error "Installation failed!"
        echo ""
        read -p "Press Enter to reboot..."
        reboot
    fi
}

# Recovery mode — automatic reflash of partitions by UUID
run_recovery_mode() {
    clear
    check_root
    check_requirements
    check_config

    if perform_recovery; then
        echo ""
        log_info "Rebooting into restored system in 3 seconds..."
        sleep 3
        reboot
    else
        echo ""
        log_error "Recovery failed!"
        log_info "Dropping to emergency shell..."
        echo ""
        PS1="[emergency] # " bash --norc
        reboot
    fi
}

# Main program — detect mode and dispatch
main() {
    if [[ -f "$RECOVERY_CONFIG" ]]; then
        run_recovery_mode
    else
        run_install_mode
    fi
}

# Run main program
main "$@"
