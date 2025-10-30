# Target Creation Workflow Planning

## Current State

Currently, creating a new target requires:
1. Manually editing `config.yaml` to add target configuration
2. Manually creating partition directory (`target-data/<target-name>/`)
3. Manually creating partition image files
4. Manually bootstrapping the OS (if applicable)

## Goal

Create a streamlined command to create new targets, especially Arch Linux-based targets.

## Proposed Command

```bash
sf create-target <target-name> [options]
```

## Design Considerations

### Approach 1: Interactive Creation
```bash
sf create-target production
# Prompts for:
# - Target type (distribution/installer)
# - Description
# - Partition layout (from template or custom)
# - Image sizes
# - Bootstrap OS? (yes/no)
```

**Pros:**
- User-friendly for new users
- Guides through all options
- Reduces errors

**Cons:**
- Slower for experienced users
- Not scriptable

### Approach 2: Template-Based Creation
```bash
sf create-target production --template arch-basic
sf create-target installer --template installer-usb
```

**Pros:**
- Fast for common use cases
- Consistent partition layouts
- Scriptable

**Cons:**
- Requires maintaining templates
- Less flexible

### Approach 3: Hybrid (Recommended)
```bash
# Quick creation with defaults
sf create-target production

# From template
sf create-target production --template arch-server

# Interactive mode
sf create-target production --interactive

# Full CLI specification
sf create-target production \
  --type distribution \
  --description "Production system" \
  --partition boot:512M:vfat:efi \
  --partition root:20G:ext4:. \
  --partition data:50G:ext4:data \
  --bootstrap arch
```

## Partition Creation Strategies

### Option 1: Empty Images
- Create sparse files of specified sizes
- Format with filesystems
- Leave empty for manual setup

**Use case:** When copying from existing system or manual setup

### Option 2: Bootstrap Arch Linux
- Use `pacstrap` to install base Arch system into mounted images
- Configure basic system (fstab, hostname, etc.)
- Install required packages

**Use case:** Creating new Arch-based targets from scratch

### Option 3: Clone Existing
- Clone from another target (we already have `sf clone-target`)
- But modify during cloning (resize, different layout)

**Use case:** Creating variants of existing targets

## Suggested Workflow for Arch Targets

### Basic Arch Target Creation
```bash
# 1. Create target with partition structure
sf create-target myarch --template arch-basic

# 2. Bootstrap Arch Linux into the images
sf bootstrap-arch myarch

# 3. Mount and customize
sf mount
sf chroot
# ... install packages, configure system ...
exit

sf unmount
```

### Advanced Arch Target Creation
```bash
# Create with custom partitions
sf create-target myarch \
  --partition boot:1G:vfat:efi \
  --partition root:30G:ext4:. \
  --partition home:100G:ext4:home \
  --partition swap:8G:swap:swap \
  --bootstrap arch \
  --packages "base linux linux-firmware networkmanager vim"
```

## Template System

Templates could be stored in `templates/` directory:

```yaml
# templates/arch-basic.yaml
name: arch-basic
description: Basic Arch Linux with boot and root partitions
type: distribution
target-data:
  - name: boot
    size: 512M
    filesystem: vfat
    mount_point: boot
    type: efi
  - name: root
    size: 20G
    filesystem: ext4
    mount_point: .
    type: linux
```

## Bootstrap Process (for Arch)

The `sf bootstrap-arch` command would:

1. Check if target exists and images are created
2. Mount partition images to temp location
3. Use `pacstrap` to install base system
4. Configure basic system:
   - Generate fstab
   - Set hostname
   - Configure locale
   - Set timezone
   - Create initramfs
   - Install bootloader (systemd-boot)
5. Unmount images
6. Target ready for `sf mount` and `sf chroot`

## Implementation Plan

### Phase 1: Basic Target Creation
- [ ] `sf create-target` with manual partition specification
- [ ] Create empty partition images
- [ ] Format filesystems
- [ ] Update config.yaml

### Phase 2: Template System
- [ ] Template directory structure
- [ ] Built-in templates (arch-basic, arch-server, installer-usb)
- [ ] Template loading and parsing
- [ ] `sf list-templates` command

### Phase 3: Arch Bootstrap
- [ ] `sf bootstrap-arch` command
- [ ] Pacstrap integration
- [ ] Basic system configuration
- [ ] Package selection options

### Phase 4: Advanced Features
- [ ] Interactive mode with prompts
- [ ] Validation of partition layouts
- [ ] Dry-run mode
- [ ] Integration with existing clone workflow

## Questions to Resolve

1. **Should partition creation be separate from target creation?**
   - Separate: `sf create-target` (config only) + `sf create-images`
   - Combined: `sf create-target` does everything

2. **How to handle image sizes?**
   - Fixed sizes in templates
   - Prompt for sizes
   - Smart defaults with override options

3. **Bootstrap requirements?**
   - Require root/sudo for pacstrap
   - Check for required tools (pacstrap, arch-install-scripts)
   - Network requirements for package downloads

4. **Image file formats?**
   - Raw sparse files (current approach)
   - Compressed images
   - Copy-on-write formats (qcow2)

5. **Should we support other distros?**
   - Start with Arch only
   - Design for extensibility (`sf bootstrap-debian`, etc.)

## Example Usage Scenarios

### Scenario 1: New Developer Setup
```bash
git clone <repo>
cd Star Forge
sf

# Create a development target from template
sf create-target dev --template arch-development
sf bootstrap-arch dev --packages "base linux vim git"
sf mount
sf chroot
# ... customize ...
```

### Scenario 2: Production Variant
```bash
# Clone existing and modify
sf clone-target dev production "Production configuration"
sf mount
sf chroot
# ... remove dev tools, add production configs ...
```

### Scenario 3: Creating Installer
```bash
# Create installer target
sf create-target prod-installer --template installer-usb

# Load production images into installer
sf load-installer production prod-installer

# Write to USB
sf write-installer /dev/sdb
```

## Next Steps

1. Review and validate approach
2. Decide on Approach (1, 2, or 3)
3. Define template format and built-in templates
4. Implement Phase 1: Basic target creation
5. Test and iterate
