# Star Forge

Star Forge is a tool for building custom Arch-based operating systems. It manages complete OS images as "targets" that you can create, customize, test in QEMU, and deploy to hardware via USB installers.

## What is a Target?

A **target** is an operating system. Each target consists of partition images (boot, root, data, etc.) stored as individual files.

```
target-data/
└── my-os/
    ├── boot.img     # EFI boot partition
    ├── root.img     # Root filesystem
    └── data.img     # Application data
```

You can have multiple targets and switch between them instantly.

## Installation

Install dependencies on Arch Linux:

```bash
sudo pacman -S util-linux e2fsprogs dosfstools parted qemu-full go-yq edk2-ovmf
```

## Basic Usage

### Create a Target

```bash
sf create my-os
```

This launches an interactive wizard that asks:
- Target type (distribution or installer)
- Partition layout (boot, root, data, etc.)
- Sizes and filesystems

### Customize Your OS

```bash
sf mount
sf chroot
```

You're now inside your OS. Install packages and configure it:

```bash
pacman -Syu
pacman -S nginx postgresql
systemctl enable nginx
vim /etc/myapp/config.yaml
```

Exit and unmount when done:

```bash
exit
sf unmount
```

### Test in QEMU

```bash
# Graphical mode
sf run

# Serial console mode
sf run-serial
```

Exit QEMU:
- **Graphical**: Close the window
- **Serial**: `Ctrl+A` then `X`

### Deploy to Hardware

Create an installer and write it to USB:

```bash
# Create installer target
sf create my-os-installer --type installer

# Load your OS into it
sf load-installer my-os my-os-installer

# Write to USB (DESTROYS ALL DATA!)
sf use my-os-installer
sf write-installer /dev/sdb
```

Boot from the USB on target hardware to install your OS.

## Commands

### Target Management

```bash
sf create <name>              # Create new target
sf clone <src> <dest>         # Clone existing target
sf list                       # List all targets
sf use <name>                 # Switch active target
sf status                     # Show current target status
sf delete <name>              # Delete a target
sf rename <old> <new>         # Rename a target
```

### Working with Targets

```bash
sf mount                      # Mount current target
sf unmount                    # Unmount partitions
sf chroot [command]           # Enter chroot or run command
```

### Testing

```bash
sf run                        # Run in QEMU (graphical)
sf run-serial                 # Run in QEMU (serial console)
```

### Deployment

```bash
sf load-installer <dist> <installer>   # Load OS into installer
sf write-installer <device>            # Write installer to USB
```

### Partition Management

```bash
sf resize-partition-image <name> <size>   # Resize partition
sf export <partition> <file>              # Export partition
sf import <file> <partition>              # Import partition
```

## Target Types

### Distribution

A complete operating system that runs on hardware. This is what you build and customize.

```bash
sf create production --type distribution
```

### Installer

A bootable USB that deploys a distribution to hardware. Contains the installer OS plus your distribution images.

```bash
sf create production-installer --type installer
sf load-installer production production-installer
```

## Configuration

Star Forge uses `config.yaml` to track targets. It's auto-managed by commands, but you can view it to understand your setup:

```yaml
current_target: "production"

targets:
  - name: production
    description: Production OS
    type: distribution
    partitions:
      - name: boot
        image: boot.img
        filesystem: vfat
        mount_point: boot
        type: efi
      - name: root
        image: root.img
        filesystem: ext4
        mount_point: .
        type: linux
```

## Example Workflows

### Building an Edge Device OS

```bash
# Create the OS
sf create edge-device

# Install your application
sf mount && sf chroot
pacman -S python python-pip
pip install myapp
systemctl enable myapp
exit && sf unmount

# Test it
sf run

# Deploy to devices
sf create edge-installer --type installer
sf load-installer edge-device edge-installer
sf use edge-installer
sf write-installer /dev/sdb
```

### Managing Multiple OS Versions

```bash
# Create production
sf create production

# Clone for development
sf clone production development

# Work on development
sf use development
sf mount && sf chroot
  # Make changes
exit && sf unmount

# Test changes
sf run

# When ready, promote to production
sf clone development production
```

### Rapid Development Cycle

```bash
# Make changes
sf mount && sf chroot
  pacman -S new-package
  systemctl enable new-service
exit && sf unmount

# Test immediately
sf run

# Repeat until satisfied
```

## How It Works

### Virtual Disks

When you run `sf run`, Star Forge combines your partition images into a bootable virtual disk using device-mapper:

```
[GPT Header] [boot.img] [root.img] [data.img] [GPT Backup]
```

QEMU boots from this virtual disk, and all changes write directly back to your partition images.

### Chroot Environment

`sf chroot` mounts your target and creates a full Arch Linux environment with access to `/proc`, `/sys`, `/dev`, and networking. You can install packages, configure services, and modify the OS as if you booted into it.

### Installers

Installers have three partitions:
1. **Boot** - EFI partition that boots the installer OS
2. **Root** - The installer OS filesystem
3. **Images** - Contains your distribution partition images + metadata

When booted on hardware, the installer can partition the target disk and write your OS images to it.

## QEMU Features

- 2GB RAM, 2 CPUs
- KVM acceleration (if available)
- VirtIO drivers for performance
- SSH port forwarding: `localhost:2222` → guest port 22
- UEFI boot support

SSH into running QEMU:
```bash
ssh user@localhost -p 2222
```

## Common Issues

**Partitions already mounted:**
```bash
sf unmount
```

**Cannot switch target:**
```bash
sf unmount
sf use <target>
```

**QEMU won't boot:**
```bash
sudo pacman -S edk2-ovmf
```

**Leftover devices after QEMU:**
```bash
sudo losetup -D
sudo dmsetup remove --force sf-run-<target>
```

## Tips

### Resize Partitions

```bash
sf unmount
sf resize-partition-image data 20G
```

### Backup Partitions

```bash
sf export root ~/backups/root-backup.img.gz
sf import ~/backups/root-backup.img.gz root
```

### Run Commands Without Interactive Chroot

```bash
sf chroot "pacman -Syu --noconfirm"
```

### Minimize OS Size

```bash
sf chroot "pacman -Scc --noconfirm"
sf unmount
sf resize-partition-image root 4G
```

### Automate Builds

```bash
#!/bin/bash
sf use production
sf mount
sf chroot "pacman -Syu --noconfirm"
sf chroot "systemctl enable myapp"
sf unmount
```

## Getting Started

```bash
# Create your first OS
sf create my-first-os

# Customize it
sf mount
sf chroot
  pacman -S vim htop
exit
sf unmount

# Test it
sf run
```
