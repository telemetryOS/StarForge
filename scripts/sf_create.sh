#!/bin/bash

# Star Forge target creation script
# Usage: sf create
# Creates a new Arch Linux target with partition images and bootstraps the system

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

print_header "Creating Target"
echo ""

check_root
check_config
check_not_mounted

# Prompt for target name
while true; do
    TARGET_NAME=$(prompt_input "Target name")
    if [[ -z "$TARGET_NAME" ]]; then
        log_error "Target name is required"
        continue
    fi

    if [[ ! "$TARGET_NAME" =~ ^[a-zA-Z0-9_-]+$ ]]; then
        log_error "Invalid target name (use only letters, numbers, dash, underscore)"
        continue
    fi

    if [[ $(find_target_index "$TARGET_NAME") != "-1" ]]; then
        log_error "Target '$TARGET_NAME' already exists"
        continue
    fi

    TARGET_DIR="$TARGET_DATA_DIR/$TARGET_NAME"
    if [[ -d "$TARGET_DIR" ]]; then
        log_error "Partition directory already exists: $TARGET_DIR"
        continue
    fi

    break
done

# Prompt for description
while true; do
    DESCRIPTION=$(prompt_input "Description")
    if [[ -z "$DESCRIPTION" ]]; then
        log_error "Description is required"
        continue
    fi
    break
done

echo ""

# Prompt for target type
log_info "Target Type"
echo "  1. distribution - Operating system for use on hardware"
echo "  2. installer    - Bootable installer for flashing distribution images"
echo ""
while true; do
    TARGET_TYPE=$(prompt_input "Target type" "1")
    case "$TARGET_TYPE" in
        1|distribution)
            TARGET_TYPE="distribution"
            break
            ;;
        2|installer)
            TARGET_TYPE="installer"
            break
            ;;
        *)
            log_error "Invalid choice. Enter 1 or 2"
            ;;
    esac
done

echo ""
log_info "Creating target: $TARGET_NAME"
log_info "Description: $DESCRIPTION"
log_info "Type: $TARGET_TYPE"
echo ""

# Step 1: Ask for root partition size
log_info "Step 1: Root Partition Size"
while true; do
    ROOT_SIZE=$(prompt_input "Root partition size" "8G")

    ROOT_SIZE="${ROOT_SIZE^^}"
    if ! numfmt --from=iec "$ROOT_SIZE" &>/dev/null; then
        log_error "Invalid size format: $ROOT_SIZE (use format like 20G, 10G, 5G)"
        continue
    fi
    break
done

echo ""

# Step 2: Partition Layout
log_info "Step 2: Partition Layout"

declare -a PARTITIONS

# Boot partition
while true; do
    BOOT_SIZE=$(prompt_input "Boot partition size" "1G")
    BOOT_SIZE="${BOOT_SIZE^^}"

    if numfmt --from=iec "$BOOT_SIZE" &>/dev/null; then
        break
    fi
    log_error "Invalid size format: $BOOT_SIZE (use format like 1G, 2G)"
done

PARTITIONS+=("boot|$BOOT_SIZE|vfat|boot|efi")

# Root partition
PARTITIONS+=("root|$ROOT_SIZE|ext4|.|linux")

# Images partition (for installer targets only)
if [[ "$TARGET_TYPE" == "installer" ]]; then
    while true; do
        IMAGES_SIZE=$(prompt_input "Images partition size" "12G")
        IMAGES_SIZE="${IMAGES_SIZE^^}"

        if numfmt --from=iec "$IMAGES_SIZE" &>/dev/null; then
            break
        fi
        log_error "Invalid size format: $IMAGES_SIZE (use format like 12G, 20G)"
    done

    PARTITIONS+=("images|$IMAGES_SIZE|ext4|images|linux")
    log_info "Added images partition: $IMAGES_SIZE"
fi

# Additional partitions
echo ""
log_info "Additional partitions (optional)"
while true; do
    if ! prompt_yesno "Add additional partition?"; then
        break
    fi

    PART_NAME=$(prompt_input "  Partition name")
    if [[ -z "$PART_NAME" ]]; then
        log_error "Partition name is required"
        continue
    fi

    PART_SIZE=$(prompt_input "  Partition size")
    if [[ -z "$PART_SIZE" ]]; then
        log_error "Partition size is required"
        continue
    fi

    PART_SIZE="${PART_SIZE^^}"
    if ! numfmt --from=iec "$PART_SIZE" &>/dev/null; then
        log_error "Invalid size format: $PART_SIZE (use format like 20G, 50G)"
        continue
    fi

    PART_MOUNT=$(prompt_input "  Mount point")
    if [[ -z "$PART_MOUNT" ]]; then
        log_error "Mount point is required"
        continue
    fi

    PART_FS="ext4"
    PART_TYPE="linux"

    PARTITIONS+=("$PART_NAME|$PART_SIZE|$PART_FS|$PART_MOUNT|$PART_TYPE")
    log_info "  Added partition: $PART_NAME (ext4)"
done

echo ""
log_info "Partition layout:"
partition_count=0
for partition in "${PARTITIONS[@]}"; do
    IFS='|' read -r name size fs mount type <<< "$partition"
    partition_count=$((partition_count + 1))
    echo "  - $name: $size [$fs] -> $mount (type: $type)"
done

echo ""
log_info "Note: When written to a device, the last partition will expand to fill available space"
echo ""

# Step 3: System Configuration
log_info "Step 3: System Configuration"

# Hostname
while true; do
    HOSTNAME=$(prompt_input "Hostname" "$TARGET_NAME")
    if [[ -z "$HOSTNAME" ]]; then
        log_error "Hostname is required"
        continue
    fi
    if [[ ! "$HOSTNAME" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$ ]]; then
        log_error "Invalid hostname format"
        continue
    fi
    break
done

# Locale
while true; do
    LOCALE=$(prompt_input "Locale" "en_US.UTF-8")
    if [[ -z "$LOCALE" ]]; then
        log_error "Locale is required"
        continue
    fi
    break
done

# Timezone
while true; do
    TIMEZONE=$(prompt_input "Timezone" "UTC")
    if [[ -z "$TIMEZONE" ]]; then
        log_error "Timezone is required"
        continue
    fi

    if ! validate_timezone "$TIMEZONE"; then
        log_error "Invalid timezone: $TIMEZONE"
        log_info "Use official timezone names (e.g., UTC, America/New_York, Europe/London)"
        log_info "Or UTC offsets (e.g., +5, -8, +5.5, -3.5)"
        continue
    fi

    break
done

# Username
while true; do
    USERNAME=$(prompt_input "First user name")
    if [[ -z "$USERNAME" ]]; then
        log_error "Username is required"
        continue
    fi
    if [[ ! "$USERNAME" =~ ^[a-z_]([a-z0-9_-]{0,31})?$ ]]; then
        log_error "Invalid username format (lowercase letters, numbers, -, _)"
        continue
    fi
    break
done

# Password
while true; do
    PASSWORD=$(prompt_password "Password for $USERNAME")
    if [[ -z "$PASSWORD" ]]; then
        log_error "Password is required"
        continue
    fi
    PASSWORD_CONFIRM=$(prompt_password "Confirm password")
    if [[ "$PASSWORD" != "$PASSWORD_CONFIRM" ]]; then
        log_error "Passwords do not match"
        continue
    fi
    break
done

echo ""
log_info "System configuration:"
log_info "  Hostname: $HOSTNAME"
log_info "  Locale: $LOCALE"
log_info "  Timezone: $TIMEZONE"
log_info "  User: $USERNAME"

echo ""
log_info "Step 4: Additional Packages"
EXTRA_PACKAGES=$(prompt_input "Additional packages for pacstrap (space-separated)" "sudo networkmanager")
echo ""

log_info "Summary:"
log_info "  Base packages: base linux linux-firmware"
if [[ -n "$EXTRA_PACKAGES" ]]; then
    log_info "  Extra packages: $EXTRA_PACKAGES"
fi

echo ""
confirm_or_exit "Create target with this configuration?"

# Create partition directory
log_info "Creating partition directory: $(relative_path "$TARGET_DIR")"
mkdir -p "$TARGET_DIR"

# Create partition images and format filesystems
log_info "Creating and formatting partition images..."
declare -a PARTITION_CONFIG

for partition in "${PARTITIONS[@]}"; do
    IFS='|' read -r name size fs mount type <<< "$partition"

    image_file="${name}.img"
    image_path="$TARGET_DIR/$image_file"

    log_info "  Creating $name ($size)..."

    truncate -s "$size" "$image_path"

    case "$fs" in
        ext4|ext3|ext2)
            log_info "    Formatting as $fs..."
            mkfs."$fs" -F "$image_path" >/dev/null 2>&1
            ;;
        vfat)
            log_info "    Formatting as vfat..."
            mkfs.vfat "$image_path" >/dev/null 2>&1
            ;;
        swap)
            log_info "    Formatting as swap..."
            mkswap "$image_path" >/dev/null 2>&1
            ;;
        *)
            log_error "Unsupported filesystem: $fs"
            exit 1
            ;;
    esac

    PARTITION_CONFIG+=("$name|$image_file|$fs|$mount|$type")
done

# Add target to config.yaml
log_info "Adding target to configuration..."

# Build partition array for yq
partition_json="["
first=true
for partition in "${PARTITION_CONFIG[@]}"; do
    IFS='|' read -r name image fs mount type <<< "$partition"

    if [[ "$first" != true ]]; then
        partition_json+=","
    fi
    first=false

    partition_json+="{\"name\":\"$name\",\"image\":\"$image\",\"filesystem\":\"$fs\",\"mount_point\":\"$mount\",\"type\":\"$type\"}"
done
partition_json+="]"

# Add target using yq
if [[ "$TARGET_TYPE" == "installer" ]]; then
    update_config ".targets += [{
        \"name\": \"$TARGET_NAME\",
        \"description\": \"$DESCRIPTION\",
        \"type\": \"$TARGET_TYPE\",
        \"images_partition\": \"images\",
        \"partitions\": $partition_json
    }]"
else
    update_config ".targets += [{
        \"name\": \"$TARGET_NAME\",
        \"description\": \"$DESCRIPTION\",
        \"type\": \"$TARGET_TYPE\",
        \"partitions\": $partition_json
    }]"
fi

log_info "Target configuration added"

# Set as current target
update_config ".current_target = \"$TARGET_NAME\""
log_info "Set as current target"

echo ""
log_info "Partition images created and formatted!"
log_info "Next: Bootstrapping Arch Linux system..."
echo ""

# Mount the partitions
log_info "Mounting partition images..."
sf mount

echo ""
log_info "Bootstrapping Arch Linux..."

# Bootstrap with pacstrap
log_info "Installing base system (this may take a few minutes)..."

# Build package list
PACKAGES="base linux linux-firmware"
if [[ -n "$EXTRA_PACKAGES" ]]; then
    PACKAGES="$PACKAGES $EXTRA_PACKAGES"
fi

if ! pacstrap "$MOUNT_DIR" $PACKAGES; then
    log_error "pacstrap failed"
    log_warn "Partitions remain mounted at $(relative_path "$MOUNT_DIR")"
    exit 1
fi

# Generate fstab
log_info "Generating fstab..."
# Generate fstab, filtering out swap entries (they come from host system)
genfstab -U "$MOUNT_DIR" | grep -v "swap" > "$MOUNT_DIR/etc/fstab"

# Set hostname
log_info "Setting hostname to: $HOSTNAME"
echo "$HOSTNAME" > "$MOUNT_DIR/etc/hostname"

# Configure locale
log_info "Configuring locale ($LOCALE)..."
echo "$LOCALE UTF-8" > "$MOUNT_DIR/etc/locale.gen"
arch-chroot "$MOUNT_DIR" locale-gen >/dev/null 2>&1
echo "LANG=$LOCALE" > "$MOUNT_DIR/etc/locale.conf"

# Set timezone
log_info "Setting timezone ($TIMEZONE)..."
NORMALIZED_TZ=$(normalize_timezone "$TIMEZONE")
arch-chroot "$MOUNT_DIR" ln -sf /usr/share/zoneinfo/$NORMALIZED_TZ /etc/localtime
arch-chroot "$MOUNT_DIR" hwclock --systohc 2>/dev/null || true

# Create user
log_info "Creating user: $USERNAME"
arch-chroot "$MOUNT_DIR" useradd -m -G wheel -s /bin/bash "$USERNAME"

# Set user password
log_info "Setting user password..."
echo "$USERNAME:$PASSWORD" | arch-chroot "$MOUNT_DIR" chpasswd

# Set root password (same as user for convenience)
log_info "Setting root password..."
echo "root:$PASSWORD" | arch-chroot "$MOUNT_DIR" chpasswd

# Enable sudo for wheel group
log_info "Configuring sudo..."
mkdir -p "$MOUNT_DIR/etc/sudoers.d"
echo "%wheel ALL=(ALL:ALL) ALL" > "$MOUNT_DIR/etc/sudoers.d/wheel"
chmod 440 "$MOUNT_DIR/etc/sudoers.d/wheel"

# Configure systemd-boot
if mountpoint -q "$MOUNT_DIR/boot" 2>/dev/null; then
    log_info "Installing systemd-boot..."

    # Install bootloader
    if ! arch-chroot "$MOUNT_DIR" bootctl install; then
        log_warn "Failed to install systemd-boot"
    else
        # Create loader configuration from templates
        log_info "Configuring boot loader..."
        mkdir -p "$MOUNT_DIR/boot/loader"
        mkdir -p "$MOUNT_DIR/boot/loader/entries"

        # Get the root partition UUID
        ROOT_PARTITION_IMAGE=$(yq -r ".targets[] | select(.name == \"$TARGET_NAME\") | .partitions[] | select(.mount_point == \".\") | .image" "$CONFIG_FILE")
        ROOT_UUID=$(blkid -s UUID -o value "$TARGET_DIR/$ROOT_PARTITION_IMAGE" 2>/dev/null || echo "UUID_NOT_FOUND")

        # Copy loader.conf template
        cp "$PROJECT_DIR/lib/bootd/default/loader.conf" "$MOUNT_DIR/boot/loader/loader.conf"

        # Copy and substitute arch.conf template
        sed "s/{{ROOT_UUID}}/$ROOT_UUID/g" "$PROJECT_DIR/lib/bootd/default/entries/arch.conf" > "$MOUNT_DIR/boot/loader/entries/arch.conf"

        log_info "Boot loader configured successfully"
    fi
fi

echo ""
log_info "Bootstrap complete!"
log_info ""
log_info "Target '$TARGET_NAME' has been created and bootstrapped"
log_info "Type: $TARGET_TYPE"
log_info "Partitions are currently mounted at: $(relative_path "$MOUNT_DIR")"
echo ""
log_info "System details:"
log_info "  User: $USERNAME (with sudo access)"
log_info "  Root and user password: (as configured)"
log_info ""

if [[ "$TARGET_TYPE" == "installer" ]]; then
    log_info "Next steps (installer target):"
    log_info "  1. Enter chroot: sf chroot"
    log_info "  2. Install installer packages and scripts"
    log_info "  3. Exit chroot: exit"
    log_info "  4. Unmount: sf unmount"
    log_info "  5. Load distribution images: sf load-installer <distribution> $TARGET_NAME"
    log_info "  6. Write to USB: sf write-installer /dev/sdX"
else
    log_info "Next steps:"
    log_info "  1. Enter chroot: sf chroot"
    log_info "  2. Install additional packages: pacman -S ..."
    log_info "  3. Configure system as needed"
    log_info "  4. Exit chroot: exit"
    log_info "  5. Unmount: sf unmount"
fi
echo ""
