#!/bin/bash

# Star Forge
# Usage: sf run [target_name]
# Runs a target in QEMU (console mode)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Parse arguments
TARGET_NAME="$1"

print_header "Run Target (Console)"

check_root

# If no target specified, use current target
if [[ -z "$TARGET_NAME" ]]; then
    TARGET_NAME=$(require_current_target)
fi

check_config
validate_target "$TARGET_NAME"

# Check not mounted
if check_is_mounted; then
    current_target=$(get_current_target)
    if [[ "$TARGET_NAME" == "$current_target" ]]; then
        log_error "Cannot run target while it is mounted"
        log_info "Unmount first with: sf unmount"
        exit 1
    fi
fi

# Get target info
target_index=$(find_target_index "$TARGET_NAME")
target_dir="$TARGET_DATA_DIR/$TARGET_NAME"

if [[ ! -d "$target_dir" ]]; then
    log_error "Target directory not found: $target_dir"
    exit 1
fi

log_info "Target: $TARGET_NAME"
log_info "Directory: $(relative_path "$target_dir")"

# Build QEMU command
partition_count=$(get_partition_count "$target_index")

# Find boot and root partitions
boot_partition=""
root_partition=""
extra_partitions=()

for i in $(seq 0 $((partition_count - 1))); do
    name=$(yq -r ".targets[$target_index].partitions[$i].name" "$CONFIG_FILE")
    image=$(yq -r ".targets[$target_index].partitions[$i].image" "$CONFIG_FILE")
    mount=$(yq -r ".targets[$target_index].partitions[$i].mount_point" "$CONFIG_FILE")

    image_path="$target_dir/$image"

    if [[ ! -f "$image_path" ]]; then
        log_error "Partition image not found: $image_path"
        exit 1
    fi

    # Normalize mount point by removing leading slash
    normalized_mount="${mount#/}"

    # Check mount point to determine partition role
    if [[ "$normalized_mount" == "boot" ]] || [[ "$normalized_mount" == "efi" ]]; then
        boot_partition="$image_path"
    elif [[ "$normalized_mount" == "." ]] || [[ -z "$normalized_mount" ]]; then
        root_partition="$image_path"
    else
        extra_partitions+=("$image_path")
    fi
done

if [[ -z "$root_partition" ]]; then
    log_error "No root partition found in target"
    exit 1
fi

# Build QEMU command
qemu_cmd="qemu-system-x86_64"
qemu_cmd="$qemu_cmd -m 2G"
qemu_cmd="$qemu_cmd -smp 2"
qemu_cmd="$qemu_cmd -nographic"

# Add EFI support if boot partition exists
if [[ -n "$boot_partition" ]]; then
    # Check if OVMF is available
    if [[ -f "/usr/share/edk2-ovmf/x64/OVMF.fd" ]]; then
        qemu_cmd="$qemu_cmd -bios /usr/share/edk2-ovmf/x64/OVMF.fd"
    elif [[ -f "/usr/share/ovmf/x64/OVMF.fd" ]]; then
        qemu_cmd="$qemu_cmd -bios /usr/share/ovmf/x64/OVMF.fd"
    else
        log_warn "OVMF BIOS not found, boot may fail"
        log_info "Install with: pacman -S edk2-ovmf"
    fi

    qemu_cmd="$qemu_cmd -drive file=$boot_partition,format=raw,if=virtio"
fi

# Add root partition
qemu_cmd="$qemu_cmd -drive file=$root_partition,format=raw,if=virtio"

# Add extra partitions
for partition in "${extra_partitions[@]}"; do
    qemu_cmd="$qemu_cmd -drive file=$partition,format=raw,if=virtio"
done

# Enable KVM if available
if [[ -w /dev/kvm ]]; then
    qemu_cmd="$qemu_cmd -enable-kvm"
else
    log_warn "KVM not available, using emulation (will be slower)"
fi

echo ""
log_info "Starting QEMU (console mode)..."
log_info "Press Ctrl+A then X to exit QEMU"
echo ""

# Run QEMU
eval $qemu_cmd
