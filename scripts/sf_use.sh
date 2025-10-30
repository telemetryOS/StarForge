#!/bin/bash

# Star Forge
# Usage: sf use [target]
# Sets the current target OS or shows available targets

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

set_target() {
    local new_target="$1"

    # Validate target exists
    if ! validate_target "$new_target"; then
        exit 1
    fi

    # Unmount if currently mounted
    if check_is_mounted; then
        log_info "Unmounting current target..."
        sf unmount
    fi

    # Update the config file
    update_config ".current_target = \"$new_target\""

    log_info "Target set to: $new_target"

    # Check if partition directory exists
    local partitions_dir="$TARGET_DATA_DIR/$new_target"
    if [[ ! -d "$partitions_dir" ]]; then
        log_warn "Partition directory not found: $(relative_path "$partitions_dir")"
        log_info "You may need to create the partition images for this target"
    else
        log_info "Partition directory: $(relative_path "$partitions_dir")"
    fi
}

check_config

if [[ $# -eq 0 ]]; then
    # No arguments - show current target and available targets
    current=$(get_current_target)
    if [[ -n "$current" ]]; then
        echo -e "${BLUE}Current target:${NC} $current"
    else
        echo -e "${YELLOW}No target currently set${NC}"
    fi
    echo ""
    echo -e "${BLUE}Available targets:${NC}"
    list_targets
else
    # Set new target
    set_target "$1"
fi