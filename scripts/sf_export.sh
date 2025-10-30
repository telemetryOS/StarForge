#!/bin/bash

# Star Forge target export script
# Usage: sf export <target_name> <output_path>
# Exports a target with its partition images and configuration to a .sftar file

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

TARGET_NAME="$1"
OUTPUT_PATH="$2"

if [[ -z "$TARGET_NAME" ]] || [[ -z "$OUTPUT_PATH" ]]; then
    echo "Usage: sf export <target_name> <output_path>"
    echo ""
    echo "Examples:"
    echo "  sf export myos /tmp              # Creates /tmp/myos.sftar"
    echo "  sf export myos /tmp/backup.sftar # Creates /tmp/backup.sftar"
    exit 1
fi

print_header "Exporting Target"

check_config

log_info "Looking for target: $TARGET_NAME"
target_index=$(find_target_index "$TARGET_NAME")

if [[ "$target_index" == "-1" ]]; then
    log_error "Target '$TARGET_NAME' not found"
    exit 1
fi

# Only prevent export if THIS specific target is currently mounted
current_target=$(get_current_target)
if [[ "$TARGET_NAME" == "$current_target" ]] && check_is_mounted; then
    log_error "Cannot export '$TARGET_NAME' while it is currently mounted"
    log_info "Unmount first with: sf unmount"
    exit 1
fi

TARGET_DIR="$TARGET_DATA_DIR/$TARGET_NAME"

if [[ ! -d "$TARGET_DIR" ]]; then
    log_error "Target directory not found: $(relative_path "$TARGET_DIR")"
    exit 1
fi

# Determine output filename
if [[ "$OUTPUT_PATH" == *.sftar ]]; then
    OUTPUT_FILE="$OUTPUT_PATH"
else
    OUTPUT_FILE="$OUTPUT_PATH/$TARGET_NAME.sftar"
fi

OUTPUT_FILE="$(realpath -m "$OUTPUT_FILE")"
OUTPUT_DIR="$(dirname "$OUTPUT_FILE")"

if [[ ! -d "$OUTPUT_DIR" ]]; then
    log_error "Output directory does not exist: $OUTPUT_DIR"
    exit 1
fi

if [[ -f "$OUTPUT_FILE" ]]; then
    log_warn "Output file already exists: $OUTPUT_FILE"
    confirm_or_exit "Overwrite?"
fi

log_info "Target: $TARGET_NAME"
log_info "Output: $OUTPUT_FILE"

total_size=$(du -sh "$TARGET_DIR" | cut -f1)
log_info "Size: $total_size"

echo ""
confirm_or_exit "Export target?"

TEMP_DIR=$(mktemp -d)
trap "rm -rf '$TEMP_DIR'" EXIT

log_info "Preparing export..."

log_info "  Extracting target configuration..."
yq ".targets[$target_index]" "$CONFIG_FILE" > "$TEMP_DIR/target.yaml"

log_info "Creating compressed archive..."
tar --zstd -cvf "$OUTPUT_FILE" \
    --transform "s|^|$TARGET_NAME/|" \
    -C "$TARGET_DIR" . \
    -C "$TEMP_DIR" target.yaml 2>&1 | \
    stdbuf -oL grep -v "^tar:" | \
    stdbuf -oL sed 's/^/  /'

FINAL_SIZE=$(du -sh "$OUTPUT_FILE" | cut -f1)

log_info ""
log_info "Export complete!"
log_info "File: $OUTPUT_FILE"
log_info "Size: $FINAL_SIZE"
