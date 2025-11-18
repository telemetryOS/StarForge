#!/bin/bash

# Star Forge
# Usage: sf run-serial [target_name]
# Runs a target in QEMU (console mode with serial output)

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

# Check if target is currently mounted
WAS_MOUNTED=false
if check_is_mounted; then
    current_target=$(get_current_target)
    if [[ "$TARGET_NAME" == "$current_target" ]]; then
        log_info "Target is currently mounted, unmounting..."
        WAS_MOUNTED=true
        sf unmount
        echo ""
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

# Set up cleanup trap before creating virtual disk
dm_name="sf-run-serial-$TARGET_NAME"

# Enhanced cleanup function that also remounts if needed
cleanup_and_remount() {
    local exit_code=$?

    # CRITICAL: Clean up virtual disk FIRST to release all loop devices
    # This ensures the boot partition image is no longer in use before we try to restore it
    cleanup_virtual_disk "$dm_name"

    # Now that all devices are released, we can safely restore boot overlay
    restore_boot_overlay

    if [[ "$WAS_MOUNTED" == "true" ]]; then
        echo ""
        log_info "Remounting target..."
        sf mount
    fi

    exit $exit_code
}

trap cleanup_and_remount EXIT
trap cleanup_and_remount INT TERM

# Apply QEMU-specific boot configuration overlay
setup_boot_overlay "$TARGET_NAME"

# Create virtual disk from partition images
create_virtual_disk "$TARGET_NAME" "$dm_name"

if [[ $? -ne 0 ]]; then
    log_error "Failed to create virtual disk"
    exit 1
fi

virtual_disk="$DM_DEVICE"

# Build QEMU command
qemu_cmd="qemu-system-x86_64"
qemu_cmd="$qemu_cmd -m 2G"
qemu_cmd="$qemu_cmd -smp 2"
qemu_cmd="$qemu_cmd -nographic"
qemu_cmd="$qemu_cmd -serial mon:stdio"

# Add EFI firmware
ovmf_path=$(get_ovmf_firmware_path)
if [[ -n "$ovmf_path" ]]; then
    qemu_cmd="$qemu_cmd -drive if=pflash,format=raw,readonly=on,file=$ovmf_path"
    log_info "Using UEFI firmware: $ovmf_path"
else
    log_warn "OVMF firmware not found, using SeaBIOS (legacy boot will fail)"
    log_info "Install with: pacman -S edk2-ovmf"
fi

# Add virtual disk (use virtio for better performance)
qemu_cmd="$qemu_cmd -drive file=$virtual_disk,format=raw,if=virtio"

# If running installer target, prepare and add test target disk
target_type=$(get_target_type "$target_index")
if [[ "$target_type" == "installer" ]]; then
    if [[ -f "$PROJECT_DIR/test/target-disk.img" ]]; then
        log_info "Wiping test target disk..."
        dd if=/dev/zero of="$PROJECT_DIR/test/target-disk.img" bs=1M count=100 conv=notrunc &>/dev/null
    else
        log_info "Creating test target disk (50G)..."
        truncate -s 50G "$PROJECT_DIR/test/target-disk.img"
    fi
    log_info "Adding test target disk for installer"
    qemu_cmd="$qemu_cmd -drive file=$PROJECT_DIR/test/target-disk.img,format=raw,if=virtio"
fi

# Enable KVM if available
if [[ -w /dev/kvm ]]; then
    qemu_cmd="$qemu_cmd -enable-kvm"
else
    log_warn "KVM not available, using emulation (will be slower)"
fi

# Add user-mode networking (built-in DHCP)
qemu_cmd="$qemu_cmd -netdev user,id=net0,hostfwd=tcp::2222-:22"
qemu_cmd="$qemu_cmd -device virtio-net-pci,netdev=net0"

echo ""
log_info "Starting QEMU (console mode)..."
log_info "Press Ctrl+A then X to exit QEMU"
echo ""

# Wait for user to be ready
read -p "Press Enter to start..."

# Save terminal state and run QEMU
stty_save=$(stty -g)
eval $qemu_cmd

# Restore terminal after QEMU exits
stty "$stty_save" 2>/dev/null || true
reset
echo ""
log_info "QEMU exited"
