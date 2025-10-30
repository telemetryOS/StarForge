#!/bin/bash

# Star Forge target import script
# Usage: sf import <sftar_file>
# Imports a target from a .sftar file, adding it to the configuration

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

SFTAR_FILE="$1"

if [[ -z "$SFTAR_FILE" ]]; then
    echo "Usage: sf import <sftar_file>"
    echo "Example: sf import /tmp/myos.sftar"
    exit 1
fi

print_header "Importing Target"

check_config

if [[ ! -f "$SFTAR_FILE" ]]; then
    log_error "File not found: $SFTAR_FILE"
    exit 1
fi

SFTAR_FILE="$(realpath "$SFTAR_FILE")"

log_info "Archive: $SFTAR_FILE"

TEMP_DIR=$(mktemp -d)
trap "rm -rf '$TEMP_DIR'" EXIT

log_info "Extracting archive..."
tar --zstd -xvf "$SFTAR_FILE" -C "$TEMP_DIR" 2>&1 | \
    stdbuf -oL grep -v "^tar:" | \
    stdbuf -oL sed 's/^/  /'

EXTRACTED_DIRS=($(ls -1 "$TEMP_DIR"))
if [[ ${#EXTRACTED_DIRS[@]} -ne 1 ]]; then
    log_error "Invalid archive: expected single directory"
    exit 1
fi

TARGET_NAME="${EXTRACTED_DIRS[0]}"
EXTRACT_DIR="$TEMP_DIR/$TARGET_NAME"

if [[ ! -f "$EXTRACT_DIR/target.yaml" ]]; then
    log_error "Invalid archive: target.yaml not found"
    exit 1
fi

log_info "Reading target configuration..."
IMPORTED_NAME=$(yq -r '.name' "$EXTRACT_DIR/target.yaml")

if [[ -z "$IMPORTED_NAME" || "$IMPORTED_NAME" == "null" ]]; then
    log_error "Invalid target configuration: missing name"
    exit 1
fi

log_info "Target name: $IMPORTED_NAME"

if [[ $(find_target_index "$IMPORTED_NAME") != "-1" ]]; then
    log_error "Target '$IMPORTED_NAME' already exists"
    log_info "Remove the existing target first with: sf delete $IMPORTED_NAME"
    exit 1
fi

TARGET_DIR="$TARGET_DATA_DIR/$IMPORTED_NAME"
if [[ -d "$TARGET_DIR" ]]; then
    log_error "Target directory already exists: $(relative_path "$TARGET_DIR")"
    exit 1
fi

DESCRIPTION=$(yq -r '.description // ""' "$EXTRACT_DIR/target.yaml")
PARTITION_COUNT=$(yq '.partitions | length' "$EXTRACT_DIR/target.yaml")

log_info "Description: $DESCRIPTION"
log_info "Partitions: $PARTITION_COUNT"

echo ""
confirm_or_exit "Import target?"

log_info "Creating target directory..."
mkdir -p "$TARGET_DIR"

log_info "Copying partition images..."
for file in "$EXTRACT_DIR"/*; do
    filename=$(basename "$file")
    if [[ "$filename" != "target.yaml" ]]; then
        cp "$file" "$TARGET_DIR/"
    fi
done

log_info "Adding target to configuration..."

TARGET_CONFIG=$(cat "$EXTRACT_DIR/target.yaml")
TARGET_CONFIG_JSON=$(echo "$TARGET_CONFIG" | yq -o=json -I=0 '.')

update_config ".targets += [$TARGET_CONFIG_JSON]"

log_info ""
log_info "Import complete!"
log_info "Target '$IMPORTED_NAME' has been imported"
log_info ""
log_info "To use this target:"
log_info "  sf set $IMPORTED_NAME"
