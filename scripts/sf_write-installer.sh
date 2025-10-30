#!/bin/bash

# Star Forge
# Usage: sf write-installer <device>
# Writes an installer target to a USB drive

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

DEVICE="$1"

if [[ -z "$DEVICE" ]]; then
    echo "Usage: sf write-installer <device>"
    echo ""
    echo "Examples:"
    echo "  sf write-installer /dev/sdb    # Write to USB drive at /dev/sdb"
    echo ""
    echo "WARNING: This will DESTROY all data on the target device!"
    exit 1
fi

print_header "Writing Installer"

check_root
check_config

# Check if partitions are currently mounted - safety check
# Writing while mounted could read inconsistent data
if check_is_mounted; then
    log_error "Cannot write installer while partitions are mounted"
    log_info "Unmount first with: sf unmount"
    exit 1
fi

# Validate device exists and is a block device
if [[ ! -b "$DEVICE" ]]; then
    log_error "Device not found or not a block device: $DEVICE"
    exit 1
fi

# Check if device is mounted
if mount | grep -q "^${DEVICE}"; then
    log_error "Device $DEVICE has mounted partitions"
    log_info "Unmount all partitions first"
    exit 1
fi

# Warn about data destruction
echo ""
log_warn "WARNING: This will DESTROY ALL DATA on $DEVICE"
log_warn "Device: $DEVICE"
device_size=$(blockdev --getsize64 "$DEVICE" 2>/dev/null || echo "0")
device_size_human=$(numfmt --to=iec-i --suffix=B $device_size)
log_warn "Size: $device_size_human"
echo ""

# Get current target
TARGET=$(require_current_target)

# Validate target is an installer
target_index=$(find_target_index "$TARGET")
if [[ "$target_index" == "-1" ]]; then
    log_error "Target '$TARGET' not found in configuration"
    exit 1
fi

target_type=$(get_target_type "$target_index")
if [[ "$target_type" != "installer" ]]; then
    log_error "Current target '$TARGET' is not an installer (type: $target_type)"
    log_info "Switch to an installer target first with: sf use <installer-target>"
    exit 1
fi

log_info "Installer target: $TARGET"

# Get partition configuration
partition_count=$(get_partition_count "$target_index")
if [[ "$partition_count" -eq 0 ]]; then
    log_error "No partitions found for target '$TARGET'"
    exit 1
fi

log_info "Partitions to write: $partition_count"
echo ""

# Verify all images exist and calculate total size
log_info "Verifying partition images..."
total_size=0
declare -a PARTITIONS
for i in $(seq 0 $((partition_count - 1))); do
    name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
    filesystem=$(yq -r ".targets[$target_index].partitions[$i].filesystem" "$CONFIG_FILE")
    part_type=$(yq -r ".targets[$target_index].partitions[$i].type" "$CONFIG_FILE")

    image_path="$TARGET_DATA_DIR/$TARGET/$image"

    if [[ ! -f "$image_path" ]]; then
        log_error "Image file not found: $image_path"
        exit 1
    fi

    size=$(stat -c%s "$image_path")
    size_mb=$((size / 1024 / 1024))
    size_human=$(numfmt --to=iec-i --suffix=B $size)
    total_size=$((total_size + size))

    PARTITIONS+=("$name|$image_path|$filesystem|$part_type|$size_mb")
    echo "  - $name: $image ($size_human)"
done

total_size_human=$(numfmt --to=iec-i --suffix=B $total_size)
log_info "Total image size: $total_size_human"

# Check if device has enough space
if [[ $total_size -gt $device_size ]]; then
    log_error "Device is too small for installer images"
    log_info "Required: $total_size_human"
    log_info "Available: $device_size_human"
    exit 1
fi

echo ""
confirm_or_exit "Proceed with writing installer to $DEVICE?"

# Unmount any partitions (just in case)
log_info "Unmounting any mounted partitions..."
umount "${DEVICE}"* 2>/dev/null || true

# Wipe partition table
log_info "Wiping partition table..."
wipefs -a "$DEVICE" >/dev/null 2>&1

# Create new GPT partition table
log_info "Creating GPT partition table..."
parted -s "$DEVICE" mklabel gpt

# Create partitions
log_info "Creating partitions..."
start_sector=2048  # Start at 2048 sectors (1MB) for alignment
sector_size=512

for i in "${!PARTITIONS[@]}"; do
    IFS='|' read -r name image_path filesystem part_type size_mb <<< "${PARTITIONS[$i]}"

    is_last=$([[ $i -eq $((${#PARTITIONS[@]} - 1)) ]] && echo "true" || echo "false")

    if [[ "$is_last" == "true" ]]; then
        # Last partition: expand to fill remaining space
        log_info "  Creating partition $((i+1)): $name (filling remaining space)"
        parted -s "$DEVICE" mkpart "$name" "${start_sector}s" "100%"
    else
        # Calculate end sector
        size_bytes=$((size_mb * 1024 * 1024))
        size_sectors=$((size_bytes / sector_size))
        end_sector=$((start_sector + size_sectors - 1))

        log_info "  Creating partition $((i+1)): $name (${size_mb}MB)"
        parted -s "$DEVICE" mkpart "$name" "${start_sector}s" "${end_sector}s"

        # Next partition starts after this one
        start_sector=$((end_sector + 1))
    fi

    # Set partition type flags
    if [[ "$part_type" == "efi" ]]; then
        parted -s "$DEVICE" set $((i+1)) esp on
    fi
done

# Wait for kernel to recognize partitions
log_info "Waiting for kernel to recognize partitions..."
partprobe "$DEVICE"
sleep 2

# Determine optimal block size for device
log_info "Determining optimal block size..."
if [[ -f "/sys/block/$(basename $DEVICE)/queue/optimal_io_size" ]]; then
    optimal_io=$(cat "/sys/block/$(basename $DEVICE)/queue/optimal_io_size")
    physical_block=$(cat "/sys/block/$(basename $DEVICE)/queue/physical_block_size")

    # Use optimal_io_size if available and non-zero, otherwise use physical block size
    if [[ $optimal_io -gt 0 ]]; then
        block_size=$optimal_io
    else
        block_size=$physical_block
    fi

    # Ensure minimum of 4K and maximum of 1M
    # USB devices often perform better with 1M than larger blocks
    if [[ $block_size -lt 4096 ]]; then
        block_size=4096
    elif [[ $block_size -gt 1048576 ]]; then
        block_size=1048576
    fi
else
    # Fallback to 1M if we can't determine optimal size
    # This is typically optimal for USB flash drives
    block_size=1048576
fi

block_size_human=$(numfmt --to=iec-i --suffix=B $block_size)
log_info "Using block size: $block_size_human ($block_size bytes)"

# Write images to partitions
log_info "Writing partition images..."
last_partition_index=$((${#PARTITIONS[@]} - 1))

for i in "${!PARTITIONS[@]}"; do
    IFS='|' read -r name image_path filesystem part_type size_mb <<< "${PARTITIONS[$i]}"

    partition_device="${DEVICE}$((i+1))"
    # Handle nvme/mmcblk devices which use p1, p2, etc
    if [[ "$DEVICE" =~ (nvme|mmcblk|loop) ]]; then
        partition_device="${DEVICE}p$((i+1))}"
    fi

    log_info "  Writing $name to $partition_device..."

    # Use pv with dd and direct I/O for accurate progress and optimal speed
    # oflag=direct bypasses kernel buffer cache for honest progress display
    # and can be faster on some USB devices by avoiding buffer flush stalls
    pv "$image_path" | dd bs="$block_size" of="$partition_device" oflag=direct status=none 2>&1

    # If this is the last partition, expand the filesystem to fill the partition
    if [[ $i -eq $last_partition_index ]]; then
        log_info "  Expanding filesystem on $name to fill partition..."

        case "$filesystem" in
            ext4|ext3|ext2)
                # Run e2fsck first
                e2fsck -f -y "$partition_device" || true
                # Expand ext filesystem
                resize2fs "$partition_device"
                log_info "  Filesystem expanded successfully"
                ;;
            vfat|fat32|fat16)
                log_warn "  Automatic expansion not supported for FAT filesystems"
                log_info "  Partition created at full size, filesystem remains at image size"
                ;;
            *)
                log_warn "  Automatic expansion not implemented for $filesystem"
                ;;
        esac
    fi
done

# Sync to ensure all writes are complete
log_info "Syncing writes to disk..."
sync

echo ""
log_info "Write complete!"
log_info "Installer '$TARGET' has been written to $DEVICE"
log_info "The device can now be used to boot and install the distribution"
