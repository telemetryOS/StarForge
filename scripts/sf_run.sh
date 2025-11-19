#!/bin/bash

# Star Forge
# Usage: sf run [target_name]
# Runs a target in QEMU (graphical mode)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Parse arguments
TARGET_NAME="$1"

print_header "Run Target (Graphical)"

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
dm_name="sf-run-$TARGET_NAME"

# Enhanced cleanup function that also remounts if needed
cleanup_and_remount() {
    local exit_code=$?
    cleanup_virtual_disk "$dm_name"

    if [[ "$WAS_MOUNTED" == "true" ]]; then
        echo ""
        log_info "Remounting target..."
        sf mount
    fi

    exit $exit_code
}

trap cleanup_and_remount EXIT
trap cleanup_and_remount INT TERM

# Create virtual disk from partition images
create_virtual_disk "$TARGET_NAME" "$dm_name"

if [[ $? -ne 0 ]]; then
    log_error "Failed to create virtual disk"
    exit 1
fi

virtual_disk="$DM_DEVICE"

# Build QEMU command
qemu_cmd="qemu-system-x86_64"

# Detect host resources and allocate reasonable amounts
# Memory: allocate 1/2 of host memory (min 2G, max 16G)
host_memory_mb=$(free -m | awk '/^Mem:/{print $2}')
guest_memory_mb=$((host_memory_mb / 2))
[[ $guest_memory_mb -lt 2048 ]] && guest_memory_mb=2048
[[ $guest_memory_mb -gt 16384 ]] && guest_memory_mb=16384
qemu_cmd="$qemu_cmd -m ${guest_memory_mb}M"
log_info "Allocating ${guest_memory_mb}MB RAM (host has ${host_memory_mb}MB)"

# CPUs: allocate half of host cores (min 2, max 8)
host_cpus=$(nproc)
guest_cpus=$((host_cpus / 2))
[[ $guest_cpus -lt 2 ]] && guest_cpus=2
[[ $guest_cpus -gt 8 ]] && guest_cpus=8
qemu_cmd="$qemu_cmd -smp $guest_cpus"
qemu_cmd="$qemu_cmd -cpu host"
log_info "Allocating $guest_cpus CPU cores (host has $host_cpus)"

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
# cache=writeback for better performance, aio=threads for async I/O
qemu_cmd="$qemu_cmd -drive file=$virtual_disk,format=raw,if=virtio,cache=writeback,aio=threads"

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
    qemu_cmd="$qemu_cmd -drive file=$PROJECT_DIR/test/target-disk.img,format=raw,if=virtio,cache=writeback,aio=threads"
fi

# Enable KVM if available
if [[ -w /dev/kvm ]]; then
    qemu_cmd="$qemu_cmd -enable-kvm"
else
    log_warn "KVM not available, using emulation (will be slower)"
fi

# Graphics related
# Dynamically allocate GPU memory: 1/6 of host RAM (min 256M, max 2G)
gpu_memory_mb=$((host_memory_mb / 4))
[[ $gpu_memory_mb -lt 256 ]] && gpu_memory_mb=256
[[ $gpu_memory_mb -gt 2048 ]] && gpu_memory_mb=2048
qemu_cmd="$qemu_cmd -device virtio-vga-gl,max_hostmem=${gpu_memory_mb}M"
qemu_cmd="$qemu_cmd -display gtk,gl=on,show-cursor=off"
log_info "Allocating ${gpu_memory_mb}MB GPU memory"

# Performance devices
qemu_cmd="$qemu_cmd -device virtio-rng-pci"        # Hardware RNG for better entropy
qemu_cmd="$qemu_cmd -device virtio-balloon-pci"    # Dynamic memory management

# Add USB tablet for absolute pointer positioning
qemu_cmd="$qemu_cmd -usb -device usb-tablet"

# Add user-mode networking (built-in DHCP)
qemu_cmd="$qemu_cmd -netdev user,id=net0,hostfwd=tcp::2222-:22"
qemu_cmd="$qemu_cmd -device virtio-net-pci,netdev=net0"

echo ""
log_info "Starting QEMU (graphical mode)..."
log_info "Close the QEMU window to exit"
echo ""

# Wait for user to be ready
read -p "Press Enter to start..."

# Run QEMU
eval $qemu_cmd

# Clean output after QEMU exits
echo ""
log_info "QEMU exited"
