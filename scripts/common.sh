#!/bin/bash

# Star Forge - Common functions and variables
# Source this in scripts: source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

COMMON_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$COMMON_SCRIPT_DIR")"
BIN_DIR="$PROJECT_DIR/bin"
TOOLS_DIR="$PROJECT_DIR/.tools"
CONFIG_FILE="$PROJECT_DIR/config.yaml"
TARGET_DATA_DIR="$PROJECT_DIR/target-data"
MOUNT_DIR="$PROJECT_DIR/mnt"

export PATH="$TOOLS_DIR/yq:$TOOLS_DIR/pv/bin:$BIN_DIR:$PATH"

log_info() {
    echo -e "$1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}$1${NC}"
}

# Usage: print_header "My Title"
print_header() {
    local title="$1"
    local title_len=${#title}
    local term_width=$(tput cols 2>/dev/null || echo 100)

    # Use a shorter line (60% of terminal width or title length + 20, whichever is smaller)
    local max_line_width=$(( term_width * 60 / 100 ))
    local min_line_width=$(( title_len + 20 ))
    local line_width=$min_line_width
    if [[ $max_line_width -lt $min_line_width ]]; then
        line_width=$max_line_width
    fi

    local line=$(printf '─%.0s' $(seq 1 $line_width))
    local title_padding=$(( (line_width - title_len) / 2 ))

    echo -e "\033[1;34m${line}\033[0m"
    printf "%${title_padding}s" ""
    echo -e "\033[1;37m${title}\033[0m"
    echo -e "\033[1;34m${line}\033[0m"
}

# Usage: print_banner "TITLE TEXT" "emoji"
print_banner() {
    local title="$1"
    local emoji="${2:-⚒}"

    local term_width=$(tput cols 2>/dev/null || echo 100)
    local line=$(printf '━%.0s' $(seq 1 $term_width))

    local full_title="$emoji  $title  $emoji"
    local title_len=${#full_title}
    local title_padding=$(( (term_width - title_len) / 2 ))

    echo -e "\033[1;36m${line}\033[0m"
    printf "%${title_padding}s" ""
    echo -e "\033[1;33m${full_title}\033[0m"
    echo -e "\033[1;36m${line}\033[0m"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run with sudo"
        exit 1
    fi
}

check_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        log_error "Configuration file not found: $CONFIG_FILE"
        exit 1
    fi
}

get_current_target() {
    local target
    target=$(yq -r '.current_target' "$CONFIG_FILE" 2>/dev/null)
    if [[ -z "$target" || "$target" == "null" ]]; then
        echo ""
        return 1
    fi
    echo "$target"
}

# Usage: find_target_index "target_name"
find_target_index() {
    local target_name="$1"
    local target_count
    local name

    target_count=$(yq '.targets | length' "$CONFIG_FILE")

    for i in $(seq 0 $((target_count - 1))); do
        name=$(yq -r ".targets[$i].name" "$CONFIG_FILE")
        if [[ "$name" == "$target_name" ]]; then
            echo "$i"
            return 0
        fi
    done

    echo "-1"
    return 0
}

list_targets() {
    local target_count
    local current_target

    target_count=$(yq '.targets | length' "$CONFIG_FILE")
    current_target=$(get_current_target)

    if [[ "$target_count" == "0" ]]; then
        echo -e "  ${YELLOW}No targets defined${NC}"
        echo -e "  Create one with: sf create"
        return
    fi

    for i in $(seq 0 $((target_count - 1))); do
        local name desc type
        name=$(yq -r ".targets[$i].name" "$CONFIG_FILE")
        desc=$(yq -r ".targets[$i].description // \"\"" "$CONFIG_FILE")
        type=$(yq -r ".targets[$i].type // \"distribution\"" "$CONFIG_FILE")

        if [[ "$name" == "$current_target" ]]; then
            echo -e "  ${GREEN}●${NC} $name [$type]: $desc ${GREEN}(current)${NC}"
        else
            echo -e "  ○ $name [$type]: $desc"
        fi
    done
}

# Usage: validate_target "target_name"
validate_target() {
    local target_name="$1"
    local index

    index=$(find_target_index "$target_name")

    if [[ "$index" == "-1" ]]; then
        log_error "Target '$target_name' not found in configuration"
        echo ""
        echo -e "${BLUE}Available targets:${NC}"
        list_targets
        return 1
    fi

    return 0
}

# Usage: get_partition_count <target_index>
get_partition_count() {
    local target_index="$1"
    yq ".targets[$target_index].partitions | length" "$CONFIG_FILE"
}

# Usage: get_target_description <target_index>
get_target_description() {
    local target_index="$1"
    yq -r ".targets[$target_index].description // \"\"" "$CONFIG_FILE"
}

# Usage: get_target_type <target_index>
get_target_type() {
    local target_index="$1"
    yq -r ".targets[$target_index].type // \"distribution\"" "$CONFIG_FILE"
}

# Usage: get_images_partition <target_index>
get_images_partition() {
    local target_index="$1"
    yq -r ".targets[$target_index].images_partition // \"\"" "$CONFIG_FILE"
}

# Usage: validate_target_type <type>
validate_target_type() {
    local type="$1"
    if [[ "$type" == "installer" || "$type" == "distribution" ]]; then
        return 0
    else
        log_error "Invalid target type: $type"
        log_info "Valid types: installer, distribution"
        return 1
    fi
}

# Usage: update_config "yq expression"
update_config() {
    local yq_expression="$1"
    local temp_config=$(mktemp)
    yq "$yq_expression" "$CONFIG_FILE" > "$temp_config"
    chown --reference="$CONFIG_FILE" "$temp_config"
    chmod --reference="$CONFIG_FILE" "$temp_config"
    mv "$temp_config" "$CONFIG_FILE"
}

# Usage: prompt_input "Prompt text" [default_value]
# Returns the user input (or default if provided and user enters nothing)
prompt_input() {
    local prompt="$1"
    local default="$2"
    local response

    if [[ -n "$default" ]]; then
        echo -ne "${CYAN}${prompt} (${default})${NC}: " >&2
    else
        echo -ne "${CYAN}${prompt}${NC}: " >&2
    fi

    read response

    if [[ -z "$response" && -n "$default" ]]; then
        echo "$default"
    else
        echo "$response"
    fi
}

# Usage: prompt_password "Prompt text"
# Returns the password (hidden input)
prompt_password() {
    local prompt="$1"
    local password

    echo -ne "${CYAN}${prompt}${NC}: " >&2
    read -s password
    echo >&2
    echo "$password"
}

# Usage: prompt_yesno "Question text"
# Returns 0 for yes, 1 for no
prompt_yesno() {
    local prompt="$1"
    echo -ne "${CYAN}${prompt}${NC} [${GREEN}y${NC}/${RED}N${NC}]: " >&2
    read -n 1 -r
    echo >&2
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        return 0
    else
        return 1
    fi
}

# Usage: confirm_or_exit ["Custom prompt"]
confirm_or_exit() {
    local prompt="${1:-Continue?}"
    echo -ne "${CYAN}${prompt}${NC} [${GREEN}y${NC}/${RED}N${NC}] " >&2
    read -n 1 -r
    echo >&2
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cancelled"
        exit 0
    fi
}

# Usage: check_not_current_target <target_name> [operation_name]
check_not_current_target() {
    local target_name="$1"
    local operation="${2:-modify}"
    local current_target=$(get_current_target)
    if [[ "$target_name" == "$current_target" ]]; then
        log_error "Cannot $operation the current target while it is active"
        log_info "Switch to a different target first with: sf use <other-target>"
        exit 1
    fi
}

# Usage: check_is_mounted
check_is_mounted() {
    if mountpoint -q "$MOUNT_DIR" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Usage: check_not_mounted [custom_message]
# Auto-unmounts if currently mounted
check_not_mounted() {
    if check_is_mounted; then
        log_info "Partitions are mounted, unmounting first..."
        sf unmount
        echo ""
    fi
}

# Usage: require_current_target
require_current_target() {
    local target
    target=$(get_current_target)
    if [[ -z "$target" ]]; then
        log_error "No current target set in configuration"

        # Check if any targets exist
        local target_count=$(yq '.targets | length' "$CONFIG_FILE" 2>/dev/null || echo "0")
        if [[ "$target_count" == "0" ]]; then
            log_info "No targets defined. Create one with: sf create"
        else
            log_info "Set target with: sf use <target>"
        fi
        exit 1
    fi
    echo "$target"
}

# Usage: relative_path <absolute_path>
relative_path() {
    local path="$1"

    # If path starts with PROJECT_DIR, make it relative
    if [[ "$path" == "$PROJECT_DIR"* ]]; then
        echo ".${path#$PROJECT_DIR}"
    else
        echo "$path"
    fi
}

# Usage: validate_timezone "timezone_string"
validate_timezone() {
    local tz="$1"

    # Check if it's a UTC offset format (+N or -N, with optional decimal)
    if [[ "$tz" =~ ^[+-][0-9]+(\.[0-9]+)?$ ]]; then
        # Extract the numeric value
        local offset="${tz#[+-]}"
        # Check if offset is within valid range (-12 to +14)
        if (( $(echo "$offset >= 0 && $offset <= 14" | bc -l) )); then
            return 0
        else
            return 1
        fi
    fi

    # Check if it's a valid timezone name from the system database
    if [[ -f "/usr/share/zoneinfo/$tz" ]]; then
        return 0
    fi

    return 1
}

# Usage: normalize_timezone "timezone_string"
normalize_timezone() {
    local tz="$1"

    # Check if it's a UTC offset format
    if [[ "$tz" =~ ^([+-])([0-9]+)(\.[0-9]+)?$ ]]; then
        local sign="${BASH_REMATCH[1]}"
        local offset="${BASH_REMATCH[2]}"
        local decimal="${BASH_REMATCH[3]}"

        # Reverse the sign for Etc/GMT format (POSIX quirk)
        if [[ "$sign" == "+" ]]; then
            sign="-"
        else
            sign="+"
        fi

        # Etc/GMT doesn't support decimal offsets, round to nearest hour
        if [[ -n "$decimal" ]]; then
            log_warn "Decimal timezone offsets not fully supported, rounding to nearest hour"
            offset=$(printf "%.0f" "$offset$decimal")
        fi

        echo "Etc/GMT${sign}${offset}"
    else
        # It's already a timezone name, return as-is
        echo "$tz"
    fi
}

# Usage: is_boot_partition "mount_point"
# Returns 0 if the mount point is a boot/EFI partition
is_boot_partition() {
    local mount="$1"
    local normalized="${mount#/}"  # Remove leading slash

    if [[ "$normalized" == "boot" ]] || [[ "$normalized" == "efi" ]]; then
        return 0
    else
        return 1
    fi
}

# Usage: is_root_partition "mount_point"
# Returns 0 if the mount point is the root partition
is_root_partition() {
    local mount="$1"
    local normalized="${mount#/}"  # Remove leading slash

    if [[ "$normalized" == "." ]] || [[ -z "$normalized" ]]; then
        return 0
    else
        return 1
    fi
}

# Usage: resolve_image_path <target_name> <image_filename>
# Resolves the full path to a partition image, checking both main directory and qemu/ subfolder
# Returns the resolved path, or empty string if not found
resolve_image_path() {
    local target_name="$1"
    local image_filename="$2"
    local target_dir="$TARGET_DATA_DIR/$target_name"

    # Check main directory first (distribution images)
    local main_path="$target_dir/$image_filename"
    if [[ -f "$main_path" ]]; then
        echo "$main_path"
        return 0
    fi

    # Check qemu/ subfolder (QEMU-only images)
    local qemu_path="$target_dir/qemu/$image_filename"
    if [[ -f "$qemu_path" ]]; then
        echo "$qemu_path"
        return 0
    fi

    # Not found in either location
    echo ""
    return 1
}

# Usage: create_virtual_disk <target_name> <dm_name>
# Creates a virtual disk device from individual partition images using device-mapper
#
# This function implements a sophisticated approach to present multiple partition images
# as a single bootable disk to QEMU. The virtual disk provides real-time bidirectional
# access - all reads and writes go directly to the partition images with no copying.
#
# Architecture:
#   1. Each partition image is mounted as a loop device
#   2. A 1MB GPT header file is created to store partition table metadata
#   3. Device-mapper (dm-linear) concatenates all pieces into a single virtual disk:
#      [GPT Header (1MB)] [Partition 1] [Partition 2] ... [Partition N] [GPT Backup (34 sectors)]
#   4. A GPT partition table is written to describe the partition layout
#   5. The kernel's partition scanner creates device nodes for each partition
#
# The result is a /dev/mapper/<dm_name> device that appears as a complete disk with
# partitions to tools like QEMU, while all data actually resides in the original
# partition image files.
#
# Global variables set:
#   LOOP_DEVICES - Array of loop devices created (for cleanup)
#   DM_DEVICE - Path to the device-mapper device
#   GPT_FILE - Path to the GPT header file (for cleanup)
create_virtual_disk() {
    local target_name="$1"
    local dm_name="$2"

    if [[ -z "$target_name" ]]; then
        log_error "create_virtual_disk: target_name is required"
        return 1
    fi

    if [[ -z "$dm_name" ]]; then
        log_error "create_virtual_disk: dm_name is required"
        return 1
    fi

    local target_index=$(find_target_index "$target_name")
    local target_dir="$TARGET_DATA_DIR/$target_name"

    if [[ "$target_index" == "-1" ]]; then
        log_error "Target not found: $target_name"
        return 1
    fi

    local partition_count=$(get_partition_count "$target_index")

    if [[ "$partition_count" -eq 0 ]]; then
        log_error "Target has no partitions"
        return 1
    fi

    # Arrays to track devices for cleanup
    declare -g -a LOOP_DEVICES=()
    declare -g DM_DEVICE="/dev/mapper/$dm_name"
    declare -g GPT_FILE=""

    log_info "Creating virtual disk from partition images..."
    log_info "Target: $target_name, Partitions: $partition_count"

    # Remove any existing device with the same name
    if dmsetup info "$dm_name" &>/dev/null; then
        log_info "Removing existing virtual disk..."

        # Check if device is mounted or in use
        local dm_device_path="/dev/mapper/$dm_name"
        if mount | grep -q "$dm_device_path"; then
            log_warn "Device is mounted, unmounting..."
            umount -f "$dm_device_path"* 2>/dev/null || true
        fi

        # Kill any processes using the device
        if command -v fuser &>/dev/null; then
            fuser -km "$dm_device_path" 2>/dev/null || true
        fi

        # Get list of loop devices used by this dm device
        local existing_loops=$(dmsetup table "$dm_name" 2>/dev/null | grep -o '/dev/loop[0-9]*' | sort -u)
        log_info "Found existing loop devices: $existing_loops"

        # IMPORTANT: Remove partition devices first (they hold the main device open)
        log_info "Removing partition devices..."
        set +e
        for part_dev in /dev/mapper/${dm_name}[0-9]*; do
            if [[ -L "$part_dev" ]]; then
                local part_name=$(basename "$part_dev")
                log_info "  Removing $part_name"
                dmsetup remove "$part_name" 2>/dev/null || true
            fi
        done
        set -e

        sleep 0.5  # Give kernel time to release partition references

        # Try to force remove dm device
        log_info "Attempting to force remove dm device: $dm_name"
        local remove_output
        set +e  # Temporarily disable exit on error
        remove_output=$(dmsetup remove --force "$dm_name" 2>&1)
        local remove_exit=$?
        set -e  # Re-enable exit on error

        log_info "dmsetup remove exit code: $remove_exit"

        if [[ $remove_exit -ne 0 ]]; then
            log_warn "dmsetup remove failed: $remove_output"

            # Try suspending then removing
            log_info "Trying suspend then remove..."
            set +e
            dmsetup suspend "$dm_name" 2>/dev/null || true
            dmsetup remove "$dm_name" 2>/dev/null || true
            set -e
        fi

        sleep 0.5  # Give kernel time to release resources

        # Then clean up loop devices
        if [[ -n "$existing_loops" ]]; then
            log_info "Detaching loop devices..."
            for loop in $existing_loops; do
                log_info "  Detaching $loop"
                losetup -d "$loop" 2>/dev/null || true
            done
        fi

        # Remove old GPT header file if exists
        rm -f "$target_dir/.gpt-header.img" 2>/dev/null || true

        # Verify removal
        if dmsetup info "$dm_name" &>/dev/null; then
            log_error "Failed to remove existing device $dm_name"
            log_error "You may need to manually remove it with: dmsetup remove --force $dm_name"
            return 1
        fi
        log_info "Existing device removed successfully"
    fi

    # Sector size (512 bytes)
    local sector_size=512
    local alignment_sectors=2048  # 1MB alignment for GPT header
    local gpt_backup_sectors=34   # Space needed for backup GPT (33 sectors + 1 for safety)
    local current_sector=$alignment_sectors

    # Create a temporary file for GPT header/footer (1MB)
    GPT_FILE="$target_dir/.gpt-header.img"
    truncate -s $((alignment_sectors * sector_size)) "$GPT_FILE"

    # Create loop device for GPT header
    local gpt_loop=$(losetup -f --show "$GPT_FILE")
    LOOP_DEVICES+=("$gpt_loop")

    # Build device-mapper table - start with GPT header space
    local dm_table="0 $alignment_sectors linear $gpt_loop 0"
    local partition_entries=""

    for i in $(seq 0 $((partition_count - 1))); do
        local name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
        local image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
        local mount=$(yq -r ".targets[$target_index].partitions[$i].mount_point" "$CONFIG_FILE")
        local filesystem=$(yq -r ".targets[$target_index].partitions[$i].filesystem" "$CONFIG_FILE")
        local image_path=$(resolve_image_path "$target_name" "$image")

        # If image not found, check if it should be auto-created in qemu/
        if [[ -z "$image_path" ]]; then
            local qemu_path="$target_dir/qemu/$image"

            # Auto-create QEMU-only images (data, recovery, fallback-recovery)
            if [[ "$name" == "data" || "$name" == "recovery" || "$name" == "fallback-recovery" ]]; then
                log_info "Creating missing QEMU image: $image"
                mkdir -p "$target_dir/qemu"

                # Determine size based on partition type
                local size
                case "$name" in
                    data)
                        size="256M"
                        ;;
                    recovery|fallback-recovery)
                        # Match root partition size
                        local root_path=$(resolve_image_path "$target_name" "root.img")
                        if [[ -n "$root_path" ]]; then
                            size=$(stat -c%s "$root_path")
                        else
                            size="6G"
                        fi
                        ;;
                esac

                # Create and format the image
                truncate -s "$size" "$qemu_path"

                case "$filesystem" in
                    ext4)
                        mkfs.ext4 -q -F "$qemu_path" >/dev/null
                        ;;
                    vfat)
                        mkfs.vfat "$qemu_path" >/dev/null
                        ;;
                esac

                image_path="$qemu_path"
                log_info "Created $image ($size, $filesystem)"
            else
                log_error "Partition image not found: $image (checked $target_dir/ and $target_dir/qemu/)"
                cleanup_virtual_disk "$dm_name"
                return 1
            fi
        fi

        # Get image size in sectors
        local image_bytes=$(stat -c%s "$image_path")
        local image_sectors=$((image_bytes / sector_size))

        log_info "  Partition $((i+1)): $name ($image_bytes bytes, $image_sectors sectors)"

        # Create loop device
        local loop_dev=$(losetup -f --show "$image_path")
        if [[ $? -ne 0 ]]; then
            log_error "Failed to create loop device for $image_path"
            cleanup_virtual_disk "$dm_name"
            return 1
        fi
        LOOP_DEVICES+=("$loop_dev")

        # Add to dm table
        dm_table+=$'\n'
        dm_table+="$current_sector $image_sectors linear $loop_dev 0"

        # Track for GPT creation
        partition_entries+="$((i + 1)):$current_sector:$image_sectors:$name:$mount "

        current_sector=$((current_sector + image_sectors))
    done

    # Add zero padding at the end for backup GPT
    dm_table+=$'\n'
    dm_table+="$current_sector $gpt_backup_sectors zero"
    current_sector=$((current_sector + gpt_backup_sectors))

    log_info "Total device size: $current_sector sectors ($((current_sector * sector_size)) bytes)"

    # Create device-mapper device
    log_info "Creating virtual disk..."
    local dm_output
    set +e  # Temporarily disable exit on error
    dm_output=$(echo "$dm_table" | dmsetup create "$dm_name" 2>&1)
    local dm_exit=$?
    set -e  # Re-enable exit on error

    if [[ $dm_exit -ne 0 ]]; then
        log_error "Failed to create device-mapper device"
        if [[ -n "$dm_output" ]]; then
            log_error "dmsetup output:"
            echo "$dm_output" >&2
        fi
        cleanup_virtual_disk "$dm_name"
        return 1
    fi

    if [[ ! -b "$DM_DEVICE" ]]; then
        log_error "Device-mapper device was created but not found: $DM_DEVICE"
        cleanup_virtual_disk "$dm_name"
        return 1
    fi

    # Write GPT partition table
    log_info "Writing GPT partition table..."

    # Create GPT partition table (suppress expected backup GPT warnings)
    if ! parted -s "$DM_DEVICE" mklabel gpt 2>/dev/null; then
        log_error "Failed to create GPT partition table"
        cleanup_virtual_disk "$dm_name"
        return 1
    fi

    # Add each partition
    local part_num=1
    for entry in $partition_entries; do
        IFS=':' read -r num start size name mount <<< "$entry"
        local end=$((start + size - 1))

        # Convert sectors to bytes for parted
        local start_bytes=$((start * sector_size))
        local end_bytes=$((end * sector_size))

        # Determine filesystem type for parted
        local fs_type="ext4"
        if is_boot_partition "$mount"; then
            fs_type="fat32"
        fi

        # Add partition (suppress expected backup GPT warnings)
        if ! parted -s "$DM_DEVICE" mkpart "$name" "$fs_type" "${start_bytes}B" "${end_bytes}B" 2>/dev/null; then
            log_error "Failed to add partition $part_num: $name"
            cleanup_virtual_disk "$dm_name"
            return 1
        fi

        # Set ESP flag for boot partition
        if is_boot_partition "$mount"; then
            if ! parted -s "$DM_DEVICE" set "$part_num" esp on 2>/dev/null; then
                log_warn "Failed to set ESP flag on partition $part_num"
            fi
        fi

        part_num=$((part_num + 1))
    done

    # Inform kernel of partition changes
    partprobe "$DM_DEVICE" 2>/dev/null || true

    log_info "Virtual disk created: $DM_DEVICE"
    return 0
}

# Usage: get_ovmf_firmware_path
# Returns the path to OVMF firmware code file, or empty string if not found
get_ovmf_firmware_path() {
    if [[ -f "/usr/share/edk2/x64/OVMF_CODE.4m.fd" ]]; then
        echo "/usr/share/edk2/x64/OVMF_CODE.4m.fd"
    elif [[ -f "/usr/share/edk2/x64/OVMF_CODE.fd" ]]; then
        echo "/usr/share/edk2/x64/OVMF_CODE.fd"
    elif [[ -f "/usr/share/edk2-ovmf/x64/OVMF_CODE.fd" ]]; then
        echo "/usr/share/edk2-ovmf/x64/OVMF_CODE.fd"
    elif [[ -f "/usr/share/OVMF/OVMF_CODE.fd" ]]; then
        echo "/usr/share/OVMF/OVMF_CODE.fd"
    else
        echo ""
    fi
}

# Usage: setup_virtual_disk_cleanup_trap <dm_name>
# Sets up trap handlers to clean up virtual disk on script exit or interrupt
setup_virtual_disk_cleanup_trap() {
    local dm_name="$1"

    # Cleanup handler that exits on interrupts
    cleanup_and_exit() {
        local exit_code=$?
        restore_boot_overlay
        cleanup_virtual_disk "$dm_name"
        exit $exit_code
    }

    # Cleanup handler for normal exit
    cleanup_all() {
        restore_boot_overlay
        cleanup_virtual_disk "$dm_name"
    }

    # Set up traps
    trap cleanup_all EXIT
    trap cleanup_and_exit INT TERM
}

# Global variables for boot config overlay
declare -g BOOT_OVERLAY_MOUNT=""
declare -g BOOT_OVERLAY_LOOP=""
declare -g BOOT_OVERLAY_BACKUP_DIR=""

# Usage: setup_boot_overlay <target_name>
# Temporarily replaces boot configs with QEMU-specific versions
# Returns 0 on success, 1 on failure
setup_boot_overlay() {
    local target_name="$1"
    local target_index=$(find_target_index "$target_name")
    local target_dir="$TARGET_DATA_DIR/$target_name"

    if [[ "$target_index" == "-1" ]]; then
        log_error "Target not found: $target_name"
        return 1
    fi

    # Find boot partition
    local partition_count=$(get_partition_count "$target_index")
    local boot_image=""

    for i in $(seq 0 $((partition_count - 1))); do
        local mount=$(yq -r ".targets[$target_index].partitions[$i].mount_point" "$CONFIG_FILE")
        if is_boot_partition "$mount"; then
            boot_image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
            break
        fi
    done

    if [[ -z "$boot_image" ]]; then
        log_warn "No boot partition found, skipping boot overlay"
        return 0
    fi

    local boot_image_path="$target_dir/$boot_image"

    # Create temporary mount point
    BOOT_OVERLAY_MOUNT=$(mktemp -d /tmp/sf-boot-overlay.XXXXXX)

    # Save boot image path for restore
    BOOT_OVERLAY_IMAGE_PATH="$boot_image_path"

    # Mount boot partition
    BOOT_OVERLAY_LOOP=$(losetup -f --show "$boot_image_path")
    if ! mount "$BOOT_OVERLAY_LOOP" "$BOOT_OVERLAY_MOUNT" 2>/dev/null; then
        log_error "Failed to mount boot partition for overlay"
        losetup -d "$BOOT_OVERLAY_LOOP" 2>/dev/null || true
        rmdir "$BOOT_OVERLAY_MOUNT" 2>/dev/null || true
        return 1
    fi

    # Create backup directory
    BOOT_OVERLAY_BACKUP_DIR=$(mktemp -d /tmp/sf-boot-backup.XXXXXX)

    # Define paths to override templates
    local qemu_overrides_dir="$PROJECT_DIR/lib/bootd/qemu"

    # Backup and replace loader.conf
    if [[ -f "$BOOT_OVERLAY_MOUNT/loader/loader.conf" ]]; then
        cp "$BOOT_OVERLAY_MOUNT/loader/loader.conf" "$BOOT_OVERLAY_BACKUP_DIR/loader.conf"

        # Copy QEMU-specific loader.conf from template
        if [[ -f "$qemu_overrides_dir/loader.conf" ]]; then
            cp "$qemu_overrides_dir/loader.conf" "$BOOT_OVERLAY_MOUNT/loader/loader.conf"
        else
            log_warn "QEMU loader.conf template not found, using inline config"
            cat > "$BOOT_OVERLAY_MOUNT/loader/loader.conf" <<EOF
default arch-qemu.conf
timeout 0
console-mode 0
editor no
EOF
        fi
    fi

    # Backup original arch.conf if it exists
    if [[ -f "$BOOT_OVERLAY_MOUNT/loader/entries/arch.conf" ]]; then
        cp "$BOOT_OVERLAY_MOUNT/loader/entries/arch.conf" "$BOOT_OVERLAY_BACKUP_DIR/arch.conf"
    fi

    # Create/update QEMU boot entry from template
    if [[ -f "$BOOT_OVERLAY_MOUNT/loader/entries/arch.conf" ]]; then
        # Get the root UUID from the original entry
        local root_uuid=$(grep -oP 'root=UUID=\K[^ ]+' "$BOOT_OVERLAY_MOUNT/loader/entries/arch.conf" || echo "")

        # Use template if available, otherwise inline
        if [[ -f "$qemu_overrides_dir/arch-qemu.conf" ]]; then
            sed "s/{{ROOT_UUID}}/$root_uuid/g" "$qemu_overrides_dir/arch-qemu.conf" > "$BOOT_OVERLAY_MOUNT/loader/entries/arch-qemu.conf"
        else
            log_warn "QEMU boot entry template not found, using inline config"
            cat > "$BOOT_OVERLAY_MOUNT/loader/entries/arch-qemu.conf" <<EOF
title   TelemetryOS Edge (QEMU)
linux   /vmlinuz-linux
initrd  /initramfs-linux.img
options root=UUID=$root_uuid rw console=ttyS0
EOF
        fi
    fi

    # Unmount and detach
    umount "$BOOT_OVERLAY_MOUNT"
    losetup -d "$BOOT_OVERLAY_LOOP"

    log_info "Boot configuration overlay applied"
    return 0
}

# Usage: restore_boot_overlay
# Restores original boot configs after QEMU exits
restore_boot_overlay() {
    if [[ -z "$BOOT_OVERLAY_BACKUP_DIR" ]] || [[ ! -d "$BOOT_OVERLAY_BACKUP_DIR" ]]; then
        return 0
    fi

    # Check if we have the boot image path saved
    if [[ -z "$BOOT_OVERLAY_IMAGE_PATH" ]] || [[ ! -f "$BOOT_OVERLAY_IMAGE_PATH" ]]; then
        log_warn "Boot overlay image path not found, skipping restore"
        rm -rf "$BOOT_OVERLAY_BACKUP_DIR" 2>/dev/null || true
        return 0
    fi

    # Re-mount boot partition
    local boot_mount=$(mktemp -d /tmp/sf-boot-restore.XXXXXX)
    local boot_loop=$(losetup -f --show "$BOOT_OVERLAY_IMAGE_PATH")

    if ! mount "$boot_loop" "$boot_mount" 2>/dev/null; then
        log_warn "Failed to mount boot partition for restore"
        losetup -d "$boot_loop" 2>/dev/null || true
        rmdir "$boot_mount" 2>/dev/null || true
        rm -rf "$BOOT_OVERLAY_BACKUP_DIR" 2>/dev/null || true
        return 1
    fi

    # Restore backed up files
    if [[ -f "$BOOT_OVERLAY_BACKUP_DIR/loader.conf" ]]; then
        cp "$BOOT_OVERLAY_BACKUP_DIR/loader.conf" "$boot_mount/loader/loader.conf"
    fi

    if [[ -f "$BOOT_OVERLAY_BACKUP_DIR/arch.conf" ]]; then
        cp "$BOOT_OVERLAY_BACKUP_DIR/arch.conf" "$boot_mount/loader/entries/arch.conf"
    fi

    # Remove dynamically created QEMU entry (since it's recreated each run)
    rm -f "$boot_mount/loader/entries/arch-qemu.conf" 2>/dev/null || true

    # Unmount and clean up
    umount "$boot_mount"
    losetup -d "$boot_loop"
    rmdir "$boot_mount"
    rm -rf "$BOOT_OVERLAY_BACKUP_DIR"

    log_info "Boot configuration restored"
    return 0
}

# Usage: cleanup_virtual_disk <dm_name>
# Tears down the virtual disk and loop devices
cleanup_virtual_disk() {
    local dm_name="$1"
    local dm_device="/dev/mapper/$dm_name"

    # Disable exit on error for cleanup
    local old_opts=$-
    set +e

    log_info "Cleaning up virtual disk..."

    # Wait a moment for any pending I/O to complete
    sync
    sleep 0.5

    # Remove partition devices first (they hold the main device open)
    if dmsetup info "$dm_name" &>/dev/null; then
        for part_dev in /dev/mapper/${dm_name}[0-9]*; do
            if [[ -L "$part_dev" ]] || [[ -b "$part_dev" ]]; then
                local part_name=$(basename "$part_dev")
                dmsetup remove "$part_name" 2>/dev/null || true
            fi
        done
        sleep 0.2
    fi

    # Remove device-mapper device
    if dmsetup info "$dm_name" &>/dev/null; then
        # Try normal remove first
        if ! dmsetup remove "$dm_name" 2>/dev/null; then
            # If still busy, wait and try force
            sleep 0.5
            dmsetup remove --force "$dm_name" 2>/dev/null || true
        fi
    fi

    # Wait for device cleanup to complete
    sleep 0.2

    # Remove loop devices (use both global array and discovery)
    if [[ ${#LOOP_DEVICES[@]} -gt 0 ]]; then
        for loop_dev in "${LOOP_DEVICES[@]}"; do
            losetup -d "$loop_dev" 2>/dev/null || true
        done
    fi

    # Also find and remove any loop devices associated with the target
    # This handles cases where LOOP_DEVICES global isn't populated
    local target_name="${dm_name#sf-run-serial-}"
    target_name="${target_name#sf-run-}"
    if [[ -n "$target_name" ]]; then
        while IFS= read -r loop_line; do
            local loop_dev=$(echo "$loop_line" | cut -d: -f1)
            if [[ -n "$loop_dev" ]] && losetup "$loop_dev" &>/dev/null; then
                losetup -d "$loop_dev" 2>/dev/null || true
            fi
        done < <(losetup -a | grep "$target_name")
    fi

    # Remove GPT header file
    if [[ -n "$GPT_FILE" ]] && [[ -f "$GPT_FILE" ]]; then
        rm -f "$GPT_FILE" 2>/dev/null || true
    fi

    # Clear global variables
    unset LOOP_DEVICES
    unset DM_DEVICE
    unset GPT_FILE

    # Restore previous shell options if 'e' was set
    if [[ $old_opts == *e* ]]; then
        set -e
    fi
}