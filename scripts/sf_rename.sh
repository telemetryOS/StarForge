#!/bin/bash

# Star Forge
# Usage: sf rename <old_name> <new_name>
# Renames a target OS configuration and its partition directory

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

OLD_NAME="$1"
NEW_NAME="$2"

if [[ -z "$OLD_NAME" ]] || [[ -z "$NEW_NAME" ]]; then
    echo "Usage: sf rename <old_name> <new_name>"
    echo "Example: sf rename staging production"
    exit 1
fi

print_header "Renaming Target"

check_config

# Check if we're trying to rename the current target
check_not_current_target "$OLD_NAME" "rename"

# Find old target
log_info "Looking for target: $OLD_NAME"
old_index=$(find_target_index "$OLD_NAME")

if [[ "$old_index" == "-1" ]]; then
    log_error "Target '$OLD_NAME' not found"
    exit 1
fi

# Check if new target name already exists
new_index=$(find_target_index "$NEW_NAME")
if [[ "$new_index" != "-1" ]]; then
    log_error "Target '$NEW_NAME' already exists"
    exit 1
fi

OLD_DIR="$TARGET_DATA_DIR/$OLD_NAME"
NEW_DIR="$TARGET_DATA_DIR/$NEW_NAME"

# Check if old directory exists
if [[ ! -d "$OLD_DIR" ]]; then
    log_warn "Partition directory not found: $OLD_DIR"
    log_info "Only configuration will be renamed"
fi

# Confirm with user
echo ""
log_warn "This will:"
log_warn "  1. Rename target in configuration: $OLD_NAME -> $NEW_NAME"
if [[ -d "$OLD_DIR" ]]; then
    log_warn "  2. Rename partition directory: $OLD_DIR -> $NEW_DIR"
fi
echo ""
confirm_or_exit

# Rename partition directory if it exists
if [[ -d "$OLD_DIR" ]]; then
    log_info "Renaming partition directory..."
    mv "$OLD_DIR" "$NEW_DIR"
fi

# Update configuration
log_info "Updating configuration..."
update_config ".targets[$old_index].name = \"$NEW_NAME\""

log_info ""
log_info "Rename complete!"
log_info "Target '$OLD_NAME' has been renamed to '$NEW_NAME'"
