#!/bin/bash

# Star Forge
# Usage: sf repair
# Runs filesystem checks and repairs on current target partition images

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

print_header "Repair Target Images"

check_root
check_config

# Get current target
TARGET=$(require_current_target)

# Check if target is mounted
check_not_mounted

target_index=$(find_target_index "$TARGET")
partition_count=$(get_partition_count "$target_index")

log_info "Target: $TARGET"
log_info "Partitions: $partition_count"
echo ""

log_warn "This will run filesystem checks and repairs on all partition images"
confirm_or_exit

TARGET_DIR="$TARGET_DATA_DIR/$TARGET"

echo ""

# Check each partition
for i in $(seq 0 $((partition_count - 1))); do
    name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
    filesystem=$(yq -r ".targets[$target_index].partitions[$i].filesystem" "$CONFIG_FILE")

    image_path="$TARGET_DIR/$image"

    if [[ ! -f "$image_path" ]]; then
        log_error "Image not found: $image_path"
        continue
    fi

    log_info "Checking $name ($filesystem)..."

    case "$filesystem" in
        ext4|ext3|ext2)
            # Check and repair ext filesystems
            # -f = force check even if clean
            # -p = automatically repair
            if e2fsck -f -p "$image_path" 2>&1; then
                log_info "  $name: OK"
            else
                exit_code=$?
                if [[ $exit_code -eq 1 ]]; then
                    log_info "  $name: Errors corrected"
                else
                    log_warn "  $name: Exit code $exit_code (see e2fsck man page)"
                fi
            fi
            ;;
        vfat|fat32|fat16)
            # Check and repair FAT filesystems
            # -a = automatically repair
            # -v = verbose
            if fsck.vfat -a -v "$image_path" 2>&1; then
                log_info "  $name: OK"
            else
                exit_code=$?
                if [[ $exit_code -eq 1 ]]; then
                    log_info "  $name: Errors corrected"
                else
                    log_warn "  $name: Exit code $exit_code"
                fi
            fi
            ;;
        *)
            log_warn "  $name: Unsupported filesystem type ($filesystem), skipping"
            ;;
    esac
    echo ""
done

log_info "Repair complete!"
log_info "All partition images have been checked and repaired"
