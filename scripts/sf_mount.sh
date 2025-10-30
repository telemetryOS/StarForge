#!/bin/bash

# Star Forge
# Usage: sf mount
# Mounts partition images to ./mnt using config.yaml config
# Requires: yq (https://github.com/mikefarah/yq)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

check_already_mounted() {
    if mountpoint -q "$MOUNT_DIR" 2>/dev/null; then
        log_error "$(relative_path "$MOUNT_DIR") is already a mount point"
        log_info "Run sf unmount first"
        exit 1
    fi
}

count_path_chunks() {
    local path="$1"
    if [[ "$path" == "." || -z "$path" ]]; then
        echo 0
    else
        echo "$path" | tr '/' '\n' | wc -l
    fi
}

print_header "Mounting Target"

check_root
check_config
check_already_mounted

log_info "Creating mount directory: $(relative_path "$MOUNT_DIR")"
mkdir -p "$MOUNT_DIR"

log_info "Reading partition configuration"

# Get current target from config
TARGET=$(require_current_target)
log_info "Using target: $TARGET"

# Find the target in the config
target_index=$(find_target_index "$TARGET")
if [[ "$target_index" == "-1" ]]; then
    log_error "Target '$TARGET' not found in configuration"
    echo ""
    echo -e "${BLUE}Available targets:${NC}"
    list_targets
    exit 1
fi

# Get number of partitions for this target
partition_count=$(get_partition_count "$target_index")

if [[ "$partition_count" -eq 0 ]]; then
    log_error "No partitions found for target '$TARGET'"
    exit 1
fi

# Check all images exist before mounting any
log_info "Verifying image files..."
for i in $(seq 0 $((partition_count - 1))); do
    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
    image_path="$TARGET_DATA_DIR/$TARGET/$image"

    if [[ ! -f "$image_path" ]]; then
        log_error "Image file not found: $image_path"
        exit 1
    fi
done

# Collect mount info
log_info "Preparing mount configuration..."
declare -A MOUNT_INFO

for i in $(seq 0 $((partition_count - 1))); do
    name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
    fs=$(yq -r ".targets[$target_index].partitions[$i].filesystem" "$CONFIG_FILE")
    mount_point=$(yq -r ".targets[$target_index].partitions[$i].mount_point" "$CONFIG_FILE")

    image_path="$TARGET_DATA_DIR/$TARGET/$image"

    depth=$(count_path_chunks "$mount_point")
    MOUNT_INFO["$depth:$mount_point"]="$image_path:$fs:$name"
done

# Mount filesystems sorted by path depth (root first, then shallow to deep)
log_info "Mounting filesystems..."

# Sort mount points by depth
sorted_keys=()
while IFS= read -r key; do
    sorted_keys+=("$key")
done < <(printf "%s\n" "${!MOUNT_INFO[@]}" | sort -t: -n -k1)

for key in "${sorted_keys[@]}"; do
    mount_point="${key#*:}"
    IFS=':' read -r image_path fs name <<< "${MOUNT_INFO[$key]}"

    if [[ "$mount_point" == "." ]]; then
        mount_point=""
    fi

    if [[ -z "$mount_point" ]]; then
        full_mount_path="$MOUNT_DIR"
    else
        full_mount_path="$MOUNT_DIR/$mount_point"
        mkdir -p "$full_mount_path"
    fi

    log_info "  Mounting $name ($fs) to $full_mount_path"
    if ! mount -o loop -t "$fs" "$image_path" "$full_mount_path"; then
        log_error "Failed to mount $name"
    fi
done

log_info ""
log_info "Mount complete!"
log_info "Root mounted at: $(relative_path "$MOUNT_DIR")"
log_info "To unmount: sf unmount"