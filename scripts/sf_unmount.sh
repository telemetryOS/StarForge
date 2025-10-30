#!/bin/bash

# Star Forge
# Usage: sf unmount
# Unmounts partition images from ./mnt

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

print_header "Unmounting Target"

check_root

# Check if anything is mounted
if [[ ! -d "$MOUNT_DIR" ]] || ! check_is_mounted; then
    log_info "Nothing appears to be mounted at $(relative_path "$MOUNT_DIR")"
    exit 0
fi

# Get list of mounted paths under MOUNT_DIR (in reverse depth order for safe unmounting)
log_info "Finding mounted filesystems..."
mounted_paths=()
while IFS= read -r mount_path; do
    if [[ "$mount_path" == "$MOUNT_DIR"* ]]; then
        mounted_paths+=("$mount_path")
    fi
done < <(mount | grep "$MOUNT_DIR" | awk '{print $3}' | sort -r)

# Unmount in reverse order (deepest first)
# Note: Loopback devices are automatically freed by the kernel when unmounting
if [[ ${#mounted_paths[@]} -gt 0 ]]; then
    log_info "Unmounting filesystems..."
    for mount_path in "${mounted_paths[@]}"; do
        log_info "  Unmounting $mount_path"
        if ! umount "$mount_path" 2>/dev/null; then
            log_warn "  Failed to unmount $mount_path - trying lazy unmount"
            umount -l "$mount_path" 2>/dev/null || true
        fi
    done
else
    log_info "No mounted filesystems found under $(relative_path "$MOUNT_DIR")"
fi

# Clean up mount directory if empty
if [[ -d "$MOUNT_DIR" ]]; then
    # Remove any empty subdirectories
    find "$MOUNT_DIR" -type d -empty -delete 2>/dev/null || true

    # Try to remove mount dir itself if empty
    rmdir "$MOUNT_DIR" 2>/dev/null && log_info "Removed empty mount directory" || true
fi

log_info ""
log_info "Unmount complete!"
