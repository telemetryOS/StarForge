#!/bin/bash
# TelemetryOS Edge Installer
# Automatically installs TelemetryOS Edge distribution images to a target disk

# Don't use set -e - we want to handle errors explicitly
set +e

# Configuration
IMAGES_DIR="/images"
PARTITIONS_CONFIG="$IMAGES_DIR/partitions.yaml"

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

    for cmd in yq blkid mkfs.ext4 mkfs.vfat parted bootctl efibootmgr resize2fs e2fsck zstd pv; do
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

    log_info "Wiping disk and creating GPT partition table..."
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

    log_info "Creating $partition_count partitions..."

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

        log_info "  Creating partition $((i + 1)): $name ($size_mb MB)"

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

# Write images to partitions
write_images() {
    local disk="$1"
    local partition_count=$(yq -r '.partitions | length' "$PARTITIONS_CONFIG")

    log_info "Writing partition images (this may take several minutes)..."
    echo ""

    for i in $(seq 0 $((partition_count - 1))); do
        local name=$(yq -r ".partitions[$i].name" "$PARTITIONS_CONFIG")
        local image=$(yq -r ".partitions[$i].image" "$PARTITIONS_CONFIG")

        local image_path="$IMAGES_DIR/$image"
        local partition="${disk}$((i + 1))"

        # Handle nvme and mmcblk devices (use p1, p2 naming)
        if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
            partition="${disk}p$((i + 1))"
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

    log_info "Rebuilding initramfs for target hardware..."

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
    log_info "  Detecting hardware and rebuilding initramfs..."
    if ! arch-chroot "$temp_mount" mkinitcpio -P 2>&1 | grep -v "^==>"; then
        log_warn "Failed to rebuild initramfs (system may not boot on this hardware)"
    else
        log_info "  Initramfs rebuilt successfully"
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

    log_info "Expanding last partition to fill disk..."

    # Use parted to resize the partition to 100%
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

    log_success "Partition expanded successfully"
    return 0
}

# Install bootloader
install_bootloader() {
    local disk="$1"

    local boot_partition="${disk}1"

    if [[ "$disk" == *"nvme"* ]] || [[ "$disk" == *"mmcblk"* ]]; then
        boot_partition="${disk}p1"
    fi

    log_info "Creating EFI boot entry..."

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
            log_info "  Created EFI boot entry"
        else
            log_warn "  Failed to create EFI entry (system may still boot via EFI fallback)"
        fi
    else
        log_warn "efibootmgr not available"
        log_info "  System should still boot via EFI fallback mechanism"
    fi

    log_success "Bootloader setup complete"
    return 0
}

# Main installation function
perform_installation() {
    local disk="$1"

    print_header "Installing TelemetryOS Edge"

    if ! initialize_disk "$disk"; then
        echo ""
        log_error "Failed to initialize disk"
        return 1
    fi

    if ! create_partitions "$disk"; then
        echo ""
        log_error "Failed to create partitions"
        return 1
    fi

    if ! write_images "$disk"; then
        echo ""
        log_error "Failed to write images"
        return 1
    fi

    if ! rebuild_initramfs "$disk"; then
        echo ""
        log_error "Failed to rebuild initramfs"
        return 1
    fi

    if ! expand_last_partition "$disk"; then
        echo ""
        log_error "Failed to expand partition"
        return 1
    fi

    if ! install_bootloader "$disk"; then
        echo ""
        log_error "Failed to install bootloader"
        return 1
    fi

    echo ""
    log_success "Installation complete!"
    return 0
}

# Main program
main() {
    clear
    print_header "TelemetryOS Edge Installer"

    echo "Welcome to TelemetryOS Edge Installer"
    echo ""
    echo "This wizard will guide you through installing TelemetryOS Edge"
    echo "to a target disk."
    echo ""

    check_root
    check_requirements
    check_config

    # Check if user wants interactive mode or automatic
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
        log_info "The system will reboot in 5 seconds..."
        log_info "Remove the USB installer now"
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

# Run main program
main "$@"