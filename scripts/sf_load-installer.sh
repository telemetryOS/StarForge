#!/bin/bash

# Star Forge
# Usage: sf load-installer <distribution-target> <installer-target>
# Copies distribution partition images into installer's images partition

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

print_header "Loading Installer"

check_root
check_config

# Check arguments
if [[ $# -ne 2 ]]; then
    log_error "Usage: sf load-installer <distribution-target> <installer-target>"
    echo ""
    echo "Example:"
    echo "  sf load-installer development development-installer"
    echo ""
    echo "This copies the distribution's partition images into the installer's images partition"
    exit 1
fi

DISTRO_TARGET="$1"
INSTALLER_TARGET="$2"

# Validate distribution target exists
log_info "Validating targets..."
if ! validate_target "$DISTRO_TARGET"; then
    exit 1
fi

# Validate installer target exists
if ! validate_target "$INSTALLER_TARGET"; then
    exit 1
fi

# Get target indices
distro_index=$(find_target_index "$DISTRO_TARGET")
installer_index=$(find_target_index "$INSTALLER_TARGET")

# Validate target types
distro_type=$(get_target_type "$distro_index")
installer_type=$(get_target_type "$installer_index")

if [[ "$distro_type" != "distribution" ]]; then
    log_error "Source target '$DISTRO_TARGET' must be of type 'distribution' (found: $distro_type)"
    exit 1
fi

if [[ "$installer_type" != "installer" ]]; then
    log_error "Destination target '$INSTALLER_TARGET' must be of type 'installer' (found: $installer_type)"
    exit 1
fi

# Get images partition name for installer
images_partition=$(get_images_partition "$installer_index")
if [[ -z "$images_partition" || "$images_partition" == "null" ]]; then
    log_error "Installer target '$INSTALLER_TARGET' does not specify an 'images_partition' field"
    exit 1
fi

log_info "Source: $DISTRO_TARGET (distribution)"
log_info "Destination: $INSTALLER_TARGET (installer)"
log_info "Images partition: $images_partition"
echo ""

# Check if anything is currently mounted
check_not_mounted

# Get list of distribution partition images
distro_partition_count=$(get_partition_count "$distro_index")
distro_dir="$TARGET_DATA_DIR/$DISTRO_TARGET"

log_info "Collecting distribution images..."
declare -a IMAGES_TO_COPY
total_size=0

for i in $(seq 0 $((distro_partition_count - 1))); do
    name=$(yq -r ".targets[$distro_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$distro_index].partitions[$i].image" "$CONFIG_FILE")
    image_path="$distro_dir/$image"

    if [[ ! -f "$image_path" ]]; then
        log_error "Distribution image not found: $image_path"
        exit 1
    fi

    size=$(stat -c%s "$image_path")
    total_size=$((total_size + size))
    size_human=$(du -h "$image_path" | cut -f1)

    IMAGES_TO_COPY+=("$image_path")
    echo "  - $name: $image ($size_human)"
done

total_size_human=$(numfmt --to=iec-i --suffix=B $total_size)
echo ""
log_info "Total size to copy: $total_size_human"

# Find the images partition in installer config
installer_partition_count=$(get_partition_count "$installer_index")
images_partition_found=false
images_partition_image=""

for i in $(seq 0 $((installer_partition_count - 1))); do
    name=$(yq -r ".targets[$installer_index].partitions[$i].name" "$CONFIG_FILE")
    if [[ "$name" == "$images_partition" ]]; then
        images_partition_found=true
        images_partition_image=$(yq -r ".targets[$installer_index].partitions[$i].image" "$CONFIG_FILE")
        images_partition_fs=$(yq -r ".targets[$installer_index].partitions[$i].filesystem" "$CONFIG_FILE")
        break
    fi
done

if [[ "$images_partition_found" != "true" ]]; then
    log_error "Images partition '$images_partition' not found in installer target configuration"
    exit 1
fi

images_partition_path="$TARGET_DATA_DIR/$INSTALLER_TARGET/$images_partition_image"
if [[ ! -f "$images_partition_path" ]]; then
    log_error "Images partition file not found: $images_partition_path"
    exit 1
fi

# Check if images partition has enough space
log_info "Checking images partition capacity..."
images_partition_size=$(stat -c%s "$images_partition_path")
images_partition_size_human=$(numfmt --to=iec-i --suffix=B $images_partition_size)

# Mount the images partition to check available space
TEMP_MOUNT=$(mktemp -d)
trap "umount '$TEMP_MOUNT' 2>/dev/null || true; rmdir '$TEMP_MOUNT' 2>/dev/null || true" EXIT

log_info "Temporarily mounting images partition..."
mount -o loop -t "$images_partition_fs" "$images_partition_path" "$TEMP_MOUNT"

available_space=$(df --output=avail "$TEMP_MOUNT" | tail -1)
available_space=$((available_space * 1024))  # Convert KB to bytes
available_space_human=$(numfmt --to=iec-i --suffix=B $available_space)

log_info "Images partition: $images_partition_size_human total, $available_space_human available"

if [[ $total_size -gt $available_space ]]; then
    umount "$TEMP_MOUNT"
    log_error "Not enough space on images partition"
    log_info "Required: $total_size_human"
    log_info "Available: $available_space_human"
    exit 1
fi

# Clean out old files from images partition
log_info "Cleaning images partition..."
if [[ $(ls -A "$TEMP_MOUNT" 2>/dev/null | wc -l) -gt 0 ]]; then
    rm -rf "$TEMP_MOUNT"/*
    log_info "  Removed old files"
else
    log_info "  Already empty"
fi

echo ""
log_info "Copying images to $TEMP_MOUNT..."

# Copy each image
for image_path in "${IMAGES_TO_COPY[@]}"; do
    image_name=$(basename "$image_path")
    log_info "  Copying $image_name..."
    cp "$image_path" "$TEMP_MOUNT/$image_name"
done

# Export partition configuration for installer
log_info "Exporting partition configuration..."
config_file="$TEMP_MOUNT/partitions.yaml"

# Initialize empty YAML with partitions array
echo "partitions: []" > "$config_file"

# Add header comment
yq -i '. head_comment = "Partition configuration exported from '"$DISTRO_TARGET"'\nGenerated by sf load-installer on '"$(date -Iseconds)"'"' "$config_file"

# Add each partition to the config using yq
for i in $(seq 0 $((distro_partition_count - 1))); do
    name=$(yq -r ".targets[$distro_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$distro_index].partitions[$i].image" "$CONFIG_FILE")
    filesystem=$(yq -r ".targets[$distro_index].partitions[$i].filesystem" "$CONFIG_FILE")
    mount_point=$(yq -r ".targets[$distro_index].partitions[$i].mount_point" "$CONFIG_FILE")
    part_type=$(yq -r ".targets[$distro_index].partitions[$i].type" "$CONFIG_FILE")

    image_path="$distro_dir/$image"
    size=$(stat -c%s "$image_path")
    size_mb=$((size / 1024 / 1024))

    yq -i ".partitions += [{
        \"name\": \"$name\",
        \"image\": \"$image\",
        \"filesystem\": \"$filesystem\",
        \"mount_point\": \"$mount_point\",
        \"type\": \"$part_type\",
        \"size_mb\": $size_mb
    }]" "$config_file"
done

log_info "  Created partitions.yaml"

# Unmount
log_info "Unmounting images partition..."
umount "$TEMP_MOUNT"
rmdir "$TEMP_MOUNT"
trap - EXIT

echo ""
log_info "Load complete!"
log_info "Distribution '$DISTRO_TARGET' images have been copied to installer '$INSTALLER_TARGET'"
log_info "Partition configuration exported to: partitions.yaml"
log_info "The installer can now deploy these images to target hardware"
