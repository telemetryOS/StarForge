#!/bin/bash

# Star Forge
# Usage: sf resize-partition-image <partition_name|image_file> <new_size>
# Examples:
#   sf resize-partition-image data 50G               # Resize 'data' partition to 50GB
#   sf resize-partition-image boot 2G                # Resize 'boot' partition to 2GB

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

IMAGE_FILE="$1"
NEW_SIZE="$2"

detect_filesystem() {
    local img="$1"

    # Try to detect filesystem using file command
    local file_output=$(file -sL "$img" 2>/dev/null)

    if [[ -z "$file_output" ]]; then
        echo "unknown"
        return 1
    fi

    if echo "$file_output" | grep -q "ext[234]"; then
        echo "ext4"
    elif echo "$file_output" | grep -q "FAT"; then
        echo "vfat"
    else
        # Try using blkid as fallback
        local fs_type=$(blkid -o value -s TYPE "$img" 2>/dev/null || true)
        if [[ -n "$fs_type" ]]; then
            echo "$fs_type"
        else
            echo "unknown"
        fi
    fi
}

if [[ -z "$IMAGE_FILE" ]] || [[ -z "$NEW_SIZE" ]]; then
    echo "Usage: sf resize-partition-image <partition_name|image_file> <new_size>"
    echo ""
    echo "Examples:"
    echo "  sf resize-partition-image data 50G              # Resize 'data' partition from current target"
    echo "  sf resize-partition-image partitions/dev/data.img 50G  # Resize specific image file"
    exit 1
fi

print_header "Resizing Partition"

check_root
check_config

# Unmount if needed - resizing a mounted image could corrupt the filesystem
check_not_mounted

# Try to resolve IMAGE_FILE as a partition name first
if [[ ! -f "$IMAGE_FILE" ]]; then
    # Not a direct file path, try to resolve as partition name
    TARGET=$(get_current_target)
    if [[ -n "$TARGET" ]]; then
        target_index=$(find_target_index "$TARGET")
        if [[ "$target_index" != "-1" ]]; then
            partition_count=$(get_partition_count "$target_index")

            # Search for partition by name
            for i in $(seq 0 $((partition_count - 1))); do
                name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
                if [[ "$name" == "$IMAGE_FILE" ]]; then
                    # Found matching partition
                    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
                    IMAGE_FILE="$TARGET_DATA_DIR/$TARGET/$image"
                    log_info "Resolved partition '$name' to: $IMAGE_FILE"
                    break
                fi
            done
        fi
    fi
fi

# Verify the final image file exists
if [[ ! -f "$IMAGE_FILE" ]]; then
    log_error "Image file not found: $IMAGE_FILE"
    log_info "Specify either a partition name from the current target or a valid image file path"
    exit 1
fi

# Get absolute path
IMAGE_FILE="$(realpath "$IMAGE_FILE" 2>/dev/null)" || {
    log_error "Cannot resolve path: $1"
    exit 1
}

IMAGE_NAME="$(basename "$IMAGE_FILE")"

log_info "Image: $IMAGE_NAME"
log_info "Target size: $NEW_SIZE"

# Detect filesystem type
FS_TYPE=$(detect_filesystem "$IMAGE_FILE")
log_info "Detected filesystem: $FS_TYPE"

if [[ "$FS_TYPE" == "unknown" ]]; then
    log_error "Could not detect filesystem type"
    exit 1
fi

# Convert new size to bytes
NEW_SIZE_BYTES=$(numfmt --from=iec "$NEW_SIZE" 2>/dev/null) || {
    log_error "Invalid size format: $NEW_SIZE (use format like 50G, 8G, 512M)"
    exit 1
}
NEW_SIZE_HUMAN=$(numfmt --to=iec "$NEW_SIZE_BYTES")

# Get current file size
CURRENT_SIZE=$(stat -c%s "$IMAGE_FILE")
CURRENT_SIZE_HUMAN=$(numfmt --to=iec "$CURRENT_SIZE")
log_info "Current file size: $CURRENT_SIZE_HUMAN"

# Handle ext filesystems
if [[ "$FS_TYPE" == "ext"* ]]; then

    # Run filesystem check - must be done before resize2fs
    log_info "Running filesystem check (this may take a moment)..."

    # resize2fs requires a forced check before expanding, so always run e2fsck -f
    if ! e2fsck -f -y "$IMAGE_FILE"; then
        log_error "Filesystem check failed"
        log_error "Manual intervention may be required"
        exit 1
    fi

    log_info "Filesystem check complete"

    # Determine if expanding or shrinking
    if [[ "$NEW_SIZE_BYTES" -gt "$CURRENT_SIZE" ]]; then
        log_info "Operation: Expand from $CURRENT_SIZE_HUMAN to $NEW_SIZE_HUMAN"

        # For expansion: grow file first, then filesystem
        log_info "Expanding image file..."
        truncate -s "$NEW_SIZE" "$IMAGE_FILE" || {
            log_error "Failed to expand image file"
            exit 1
        }

        log_info "Expanding filesystem..."
        resize2fs "$IMAGE_FILE" || {
            log_error "Failed to expand filesystem"
            exit 1
        }

    elif [[ "$NEW_SIZE_BYTES" -lt "$CURRENT_SIZE" ]]; then
        log_info "Operation: Shrink from $CURRENT_SIZE_HUMAN to $NEW_SIZE_HUMAN"

        # Check minimum size
        log_info "Checking minimum filesystem size..."
        RESIZE_OUTPUT=$(resize2fs -P "$IMAGE_FILE" 2>&1)
        if [[ $? -ne 0 ]]; then
            log_error "Failed to determine minimum size:"
            log_error "$RESIZE_OUTPUT"
            exit 1
        fi

        # Parse the output - resize2fs outputs: "Estimated minimum size of the filesystem: NNNNNN"
        MIN_BLOCKS=$(echo "$RESIZE_OUTPUT" | grep -oE '[0-9]+' | tail -1)
        if [[ -z "$MIN_BLOCKS" ]]; then
            log_error "Could not parse minimum size from: $RESIZE_OUTPUT"
            exit 1
        fi

        MIN_SIZE=$((MIN_BLOCKS * 4096))
        MIN_SIZE_HUMAN=$(numfmt --to=iec "$MIN_SIZE")

        if [[ "$NEW_SIZE_BYTES" -lt "$MIN_SIZE" ]]; then
            log_error "Cannot shrink below minimum size: $MIN_SIZE_HUMAN"
            exit 1
        fi

        # For shrinking: shrink filesystem first, then file
        log_info "Shrinking filesystem to $NEW_SIZE_HUMAN (this may take a while)..."

        # Use -f to force and avoid prompts
        if ! resize2fs -f "$IMAGE_FILE" "$NEW_SIZE"; then
            log_error "Failed to shrink filesystem"
            log_warn "The operation may have been interrupted or failed"
            exit 1
        fi

        log_info "Filesystem shrink complete"

        log_info "Shrinking image file..."
        truncate -s "$((NEW_SIZE_BYTES + 1048576))" "$IMAGE_FILE" || {
            log_error "Failed to shrink image file"
            exit 1
        }
    else
        log_info "Image is already the requested size"
        exit 0
    fi

    # Final verification
    log_info "Verifying filesystem..."
    if ! e2fsck -f -n "$IMAGE_FILE" &>/dev/null; then
        log_warn "Running final repair..."
        e2fsck -f -y "$IMAGE_FILE" || true
    fi

# Handle FAT filesystems
elif [[ "$FS_TYPE" == "vfat" ]] || [[ "$FS_TYPE" == "fat"* ]]; then
    log_error "FAT filesystems cannot be resized"
    log_info "FAT partitions (like boot/EFI) are typically small and don't need resizing"
    exit 1

else
    log_error "Unsupported filesystem type: $FS_TYPE"
    exit 1
fi

# Get final size
FINAL_SIZE=$(stat -c%s "$IMAGE_FILE")
FINAL_SIZE_HUMAN=$(numfmt --to=iec "$FINAL_SIZE")

log_info ""
log_info "Resize complete!"
log_info "Final size: $FINAL_SIZE_HUMAN"
log_info "You can test with: sudo mount -o loop $IMAGE_FILE /mnt/test"