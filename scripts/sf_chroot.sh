#!/bin/bash

# Star Forge
# Usage: sf chroot [command]
# Enters chroot environment in the mounted OS

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

print_header "Change Root to Target"

check_root

# Auto-mount if not already mounted
if ! mountpoint -q "$MOUNT_DIR" 2>/dev/null; then
    log_info "Target is not mounted, mounting first..."
    sf mount
    echo ""
fi

# Use arch-chroot to enter the environment
if [[ $# -gt 0 ]]; then
    log_info "Executing command in chroot: $*"
    arch-chroot "$MOUNT_DIR" /bin/sh -c "$*"
else
    log_info "Entering chroot environment at $(relative_path "$MOUNT_DIR")"
    log_info "Type 'exit' to leave the chroot"
    echo ""

    arch-chroot "$MOUNT_DIR"
fi

log_info "Exited chroot environment"