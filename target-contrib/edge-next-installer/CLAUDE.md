# TelemetryOS Edge Installer

## Overview
Bootable USB installer that deploys TelemetryOS Edge to target hardware. Also supports recovery mode (reflashing from on-disk recovery partitions).

## Boot Chain
systemd (multi-user.target) → getty autologin as `root` → `.bash_profile` → `exec /install.sh`

## USB Partition Layout
- **Partition 1** (1G vfat): boot — kernel, initramfs, systemd-boot entries
- **Partition 2** (4G ext4): installer root — install.sh, system files
- **Partition 3** (23.7G ext4): images — compressed edge images (.zst) + partitions.yaml

## Update Pipeline: contrib → images → USB

```bash
# 1. Mount USB partitions (check device with lsblk — flips between sda/sdb)
sudo mount /dev/sdX1 /tmp/usb-boot
sudo mount /dev/sdX2 /tmp/usb-root
sudo mount /dev/sdX3 /tmp/usb-images

# 2. Compress edge images to USB
sudo zstd -T0 -3 -f target-data/edge/boot.img -o /tmp/usb-images/boot.img.zst
sudo zstd -T0 -3 -f target-data/edge/root.img -o /tmp/usb-images/root.img.zst

# 3. Copy installer contrib to USB
sudo cp -a target-contrib/edge-installer/* /tmp/usb-root/
sudo cp -a target-contrib/edge-installer/boot/* /tmp/usb-boot/

# 4. Fix boot entry UUID placeholders
ROOT_UUID=$(blkid -s UUID -o value /dev/sdX2)
sudo sed -i "s|{{ROOT_UUID}}|$ROOT_UUID|g" /tmp/usb-boot/loader/entries/*.conf

# 5. Sync before pulling USB
sudo sync
```

## Boot Entry UUID Placeholders
Boot entries in `boot/loader/entries/*.conf` use `{{ROOT_UUID}}` as a placeholder. This MUST be replaced with the actual USB root partition UUID every time contrib is copied to the boot partition. Forgetting this causes the installer to fail to boot.

## Install Script (install.sh)

### Error Handling
Uses `set +e` — errors don't abort. Functions return 1 on failure, callers check with `|| return 1`.

### Key Design Constraints
- **Never modify partitions during `write_images()`** — mounting/chowning during image writing caused EGL/Sway failures on the installed system
- **Partition permissions** (owner/group/mode from partitions.yaml) are applied in `configure_uuids()` where the target root is mounted and user/group names resolve from the **target's** `/etc/passwd`, not the installer's
- **yq (Mike Farah v4)**: Use `// ""` for null fallback, NOT `// empty`
- **mkfs force flags**: `mkfs.ext4 -qF`, `mkfs.vfat -F 32` to skip confirmation prompts

### Installation Steps (perform_installation)
1. Initialize disk (GPT partition table)
2. Create partitions (from partitions.yaml)
3. Write partition images (dd compressed images, mkfs for empty partitions)
4. Rebuild initramfs (arch-chroot, mkinitcpio)
5. Expand last partition (resize to fill disk)
6. Configure UUIDs (fstab, boot entries, partition permissions)
7. Install bootloader (systemd-boot)
8. Populate recovery partitions

### partitions.yaml
Lives on USB partition 3. Supports optional `owner`, `group`, `mode` fields for empty partitions:
```yaml
partitions:
  - name: data
    image: empty
    filesystem: ext4
    mount_point: data
    type: linux
    size_mb: 256
    owner: data
    group: data
    mode: "2775"
```

## Key Config Files
- `install.sh` — main installer script (must be executable)
- `boot/loader/entries/arch.conf` — boot entry with {{ROOT_UUID}} placeholder
- `boot/loader/entries/arch-qemu.conf` — QEMU serial boot entry
- `etc/systemd/system/getty@tty1.service.d/autologin.conf` — root autologin
- `etc/systemd/system/default.target` — symlink to multi-user.target
