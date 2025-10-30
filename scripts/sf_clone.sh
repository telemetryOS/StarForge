#!/bin/bash

# Star Forge
# Usage: sf clone <source_target> <new_target> [description]
# Clones a target OS configuration and partition images

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

SOURCE_TARGET="$1"
NEW_TARGET="$2"
NEW_DESCRIPTION="${3:-Cloned from $SOURCE_TARGET}"

if [[ -z "$SOURCE_TARGET" ]] || [[ -z "$NEW_TARGET" ]]; then
    echo "Usage: sf clone <source_target> <new_target> [description]"
    echo "Example: sf clone development production 'Production OS image'"
    exit 1
fi

print_header "Cloning Target"

check_config

# Check if partitions are currently mounted - safety check
# Cloning while mounted could copy inconsistent data
if check_is_mounted; then
    log_error "Cannot clone targets while partitions are mounted"
    log_info "Unmount first with: sf unmount"
    exit 1
fi

# Find source target
log_info "Looking for source target: $SOURCE_TARGET"
source_index=$(find_target_index "$SOURCE_TARGET")

if [[ "$source_index" == "-1" ]]; then
    log_error "Source target '$SOURCE_TARGET' not found"
    exit 1
fi

# Check if new target already exists
new_target_check=$(find_target_index "$NEW_TARGET")
if [[ "$new_target_check" != "-1" ]]; then
    log_error "Target '$NEW_TARGET' already exists"
    exit 1
fi

SOURCE_DIR="$TARGET_DATA_DIR/$SOURCE_TARGET"
NEW_DIR="$TARGET_DATA_DIR/$NEW_TARGET"

# Check source directory exists
if [[ ! -d "$SOURCE_DIR" ]]; then
    log_error "Source partition directory not found: $SOURCE_DIR"
    exit 1
fi

# Check if destination already exists
if [[ -d "$NEW_DIR" ]]; then
    log_error "Destination directory already exists: $NEW_DIR"
    exit 1
fi

# Get partition count
partition_count=$(get_partition_count "$source_index")
log_info "Source has $partition_count partition(s)"

# Calculate total size
log_info "Calculating partition sizes..."
total_size=0
for i in $(seq 0 $((partition_count - 1))); do
    image=$(yq -r ".targets[$source_index].partitions[$i].image" "$CONFIG_FILE")
    image_path="$SOURCE_DIR/$image"

    if [[ ! -f "$image_path" ]]; then
        log_error "Source image not found: $image_path"
        exit 1
    fi

    size=$(stat -c%s "$image_path")
    total_size=$((total_size + size))
    size_human=$(numfmt --to=iec "$size")
    log_info "  $image: $size_human"
done

total_human=$(numfmt --to=iec "$total_size")
log_info "Total size to copy: $total_human"

# Confirm with user
echo ""
log_warn "This will:"
log_warn "  1. Create directory: $NEW_DIR"
log_warn "  2. Copy all partition images ($total_human)"
log_warn "  3. Add new target '$NEW_TARGET' to configuration"
echo ""
confirm_or_exit

# Create new directory
log_info "Creating directory: $NEW_DIR"
mkdir -p "$NEW_DIR"

# Copy partition images
log_info "Copying partition images..."
for i in $(seq 0 $((partition_count - 1))); do
    image=$(yq -r ".targets[$source_index].partitions[$i].image" "$CONFIG_FILE")
    name=$(yq -r ".targets[$source_index].partitions[$i].name" "$CONFIG_FILE")

    log_info "  Copying $name ($image)..."
    cp "$SOURCE_DIR/$image" "$NEW_DIR/$image"
done

log_info "All images copied successfully"

# Extract source target configuration
log_info "Adding target to configuration..."
source_config=$(yq ".targets[$source_index]" "$CONFIG_FILE")

# Create new target config with updated name and description
new_config=$(echo "$source_config" | yq ".name = \"$NEW_TARGET\" | .description = \"$NEW_DESCRIPTION\"")

# Append to config file
update_config ".targets += [$new_config]"

log_info ""
log_info "Clone complete!"
log_info "New target '$NEW_TARGET' created"
log_info "Partition directory: $NEW_DIR"
log_info ""
log_info "To use this target:"
log_info "  sf use $NEW_TARGET"