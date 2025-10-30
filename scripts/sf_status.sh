#!/bin/bash

# Star Forge
# Usage: sf status
# Shows current mount status of partition images

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

print_header "System Status"
echo ""

check_config

# Check mount status
echo -e "${BLUE}Mount Status:${NC}"
if check_is_mounted; then
    echo -e "  ${GREEN}✓${NC} Root mounted at: $MOUNT_DIR"
else
    echo -e "  ${YELLOW}○${NC} Partitions are not mounted"
fi
echo ""

# Show mounted filesystems
echo -e "${BLUE}Mounted Filesystems:${NC}"
mounted_count=0
while IFS= read -r line; do
    if [[ "$line" == *"$MOUNT_DIR"* ]]; then
        mount_point=$(echo "$line" | awk '{print $3}')
        device=$(echo "$line" | awk '{print $1}')
        fs_type=$(echo "$line" | awk -F'[()]' '{print $2}' | cut -d, -f1)

        # Make path relative to MOUNT_DIR for display
        rel_path="${mount_point#$MOUNT_DIR}"
        if [[ -z "$rel_path" ]]; then
            rel_path="/ (root)"
        else
            rel_path="$rel_path"
        fi

        echo -e "  ${GREEN}✓${NC} $rel_path [$fs_type] on $device"
        ((mounted_count++))
    fi
done < <(mount)

if [[ $mounted_count -eq 0 ]]; then
    echo "  None"
fi
echo ""

# Show current target
echo -e "${BLUE}Current Target:${NC}"
TARGET=$(get_current_target)
if [[ -z "$TARGET" ]]; then
    echo -e "  ${YELLOW}No target set${NC}"
    echo -e "  Set with: sf use <target>"
    echo ""
    echo -e "${BLUE}Available targets:${NC}"
    list_targets
    exit 0
fi

# Find target index
target_index=$(find_target_index "$TARGET")
if [[ "$target_index" == "-1" ]]; then
    echo -e "  ${RED}Target '$TARGET' not found in configuration${NC}"
    exit 1
fi

target_desc=$(get_target_description "$target_index")
target_type=$(get_target_type "$target_index")
echo -e "  ${GREEN}$TARGET${NC}: $target_desc"
echo -e "  Type: $target_type"
echo ""

# Show partition configuration for current target
echo -e "${BLUE}Partitions for $TARGET:${NC}"
partition_count=$(get_partition_count "$target_index")
for i in $(seq 0 $((partition_count - 1))); do
    name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
    fs=$(yq -r ".targets[$target_index].partitions[$i].filesystem" "$CONFIG_FILE")
    mount_point=$(yq -r ".targets[$target_index].partitions[$i].mount_point" "$CONFIG_FILE")

    image_path="$TARGET_DATA_DIR/$TARGET/$image"

    # Check if image exists
    if [[ -f "$image_path" ]]; then
        size=$(du -h "$image_path" | cut -f1)
        status="${GREEN}✓${NC}"
        details="$size"
    else
        status="${RED}✗${NC}"
        details="missing"
    fi

    echo -e "  $status $name: $image [$fs] -> $mount_point ($details)"
done
echo ""

# Summary
echo -e "${BLUE}Summary:${NC}"
if check_is_mounted && [[ $mounted_count -gt 0 ]]; then
    echo -e "  ${GREEN}System is mounted and ready${NC}"
    echo "  To unmount: sf unmount"
else
    echo -e "  ${YELLOW}System is not mounted${NC}"
    echo "  To mount: sf mount"
fi