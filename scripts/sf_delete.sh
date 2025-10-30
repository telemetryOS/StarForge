#!/bin/bash

# Star Forge target deletion script
# Usage: sf delete <target_name>
# Deletes a target OS configuration and its partition directory

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

TARGET_NAME="$1"

if [[ -z "$TARGET_NAME" ]]; then
    echo "Usage: sf delete <target_name>"
    echo "Example: sf delete old-staging"
    exit 1
fi

print_header "Deleting Target"

check_config

# Find target
log_info "Looking for target: $TARGET_NAME"
target_index=$(find_target_index "$TARGET_NAME")

if [[ "$target_index" == "-1" ]]; then
    log_error "Target '$TARGET_NAME' not found"
    exit 1
fi

# Check if this is the current target - if so, find another to switch to
current_target=$(get_current_target)
is_current=false
new_target=""

if [[ "$TARGET_NAME" == "$current_target" ]]; then
    is_current=true
    log_warn "Target '$TARGET_NAME' is currently active"

    target_count=$(yq '.targets | length' "$CONFIG_FILE")
    for i in $(seq 0 $((target_count - 1))); do
        if [[ $i -eq $target_index ]]; then
            continue  # Skip the target we're deleting
        fi
        name=$(yq -r ".targets[$i].name" "$CONFIG_FILE")
        new_target="$name"
        break
    done

    log_info "Will switch to target: $new_target"
fi

TARGET_DIR="$TARGET_DATA_DIR/$TARGET_NAME"

# Calculate size if directory exists
if [[ -d "$TARGET_DIR" ]]; then
    total_size=$(du -sh "$TARGET_DIR" | cut -f1)
    log_info "Partition directory: $(relative_path "$TARGET_DIR") ($total_size)"
else
    log_warn "Partition directory not found: $(relative_path "$TARGET_DIR")"
    log_info "Only configuration will be removed"
fi

# Confirm with user
echo ""
log_warn "This will:"
log_warn "  1. Remove target from configuration: $TARGET_NAME"
if [[ -d "$TARGET_DIR" ]]; then
    log_warn "  2. Delete partition directory: $(relative_path "$TARGET_DIR") ($total_size)"
    log_warn "     WARNING: This will permanently delete all partition images!"
fi
echo ""
confirm_or_exit

# Unmount and switch to new target if removing current
if [[ "$is_current" == true ]]; then
    if check_is_mounted; then
        log_info "Unmounting current target..."
        sf unmount
    fi

    log_info "Switching to target: $new_target"
    update_config ".current_target = \"$new_target\""
fi

# Remove partition directory if it exists
if [[ -d "$TARGET_DIR" ]]; then
    log_info "Removing partition directory..."
    rm -rf "$TARGET_DIR"
fi

# Remove from configuration
log_info "Updating configuration..."
update_config "del(.targets[$target_index])"

log_info ""
log_info "Removal complete!"
log_info "Target '$TARGET_NAME' has been removed"
if [[ "$is_current" == true ]]; then
    log_info "Current target is now: $new_target"
fi
