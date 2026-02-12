---
title: "Deploying"
weight: 16
---

After building a target, StarForge provides several ways to deploy the resulting image: writing directly to a device, exporting a full disk image, or exporting individual partition images.

## Writing to Devices

The [`starforge write`](../../commands/write/) command writes a built target directly to a storage device such as a USB drive, SD card, or internal disk:

```bash
starforge write distribution /dev/sdb
```

You will be prompted to confirm before any data is written. The confirmation shows the target device and its current partition table to help prevent accidental data loss.

The write operation targets the **whole device** (e.g., `/dev/sdb`), not an individual partition. StarForge creates a GPT partition table and writes each partition image to its corresponding device partition.

### Growable Partition Expansion

Partitions defined with a `+` suffix on their size (e.g., `256M+`, `8G+`) are growable. When writing to a device, growable partitions expand to fill the remaining disk space. This allows a single image to work on devices of different sizes -- the growable partition absorbs the extra space.

For example, a partition defined as `256M+` will use at least 256 MB during the build but expand to fill all remaining space when written to a 32 GB USB drive.

```yaml
# In the layer definition
- action: partition-add
  partitions:
    - name: data
      filesystem: ext4
      size: 256M+
      mount_point: /data
      type: linux
```

## Exporting Disk Images

The [`starforge export`](../../commands/export/) command with the `disk` subcommand creates a single bootable `.img` file:

```bash
starforge export distribution disk --size 16G --output ./release/my-os.img
```

The `--size` flag specifies the total disk image size. Growable partitions expand to fill the allocated space, just as they do when writing to a physical device.

The `--output` flag sets the destination path. If the output directory does not exist, it is created.

The resulting image can be flashed to a device with `dd`:

```bash
sudo dd if=./release/my-os.img of=/dev/sdX bs=4M status=progress
```

Or distributed for flashing with any disk imaging tool (Etcher, Raspberry Pi Imager, etc.).

## Exporting Partition Images

The `partitions` subcommand exports individual partition images rather than a single disk image:

```bash
starforge export distribution partitions --output ./release/
```

This produces separate files for each partition:

```
release/
├── boot.img
├── root.img
└── data.img
```

Individual partition images are useful for:

- **OTA update systems** that update specific partitions without reflashing the entire disk
- **Custom deployment pipelines** that assemble disk images from partition components
- **A/B update schemes** where only the active partition is swapped

## Cleaning Up

The [`starforge clean`](../../commands/clean/) command removes build artifacts. It accepts a target name and an optional scope to control what is removed.

### Clean All Artifacts for a Target

Remove everything -- cache, partition images, and overlays:

```bash
starforge clean distribution
```

### Clean Specific Scopes

Remove only the overlay cache (forces all phases to rebuild on next build):

```bash
starforge clean distribution cache
```

Remove only the partition images:

```bash
starforge clean distribution images
```

Remove only the exported disk images:

```bash
starforge clean distribution disks
```

### Clean Vendored Dependencies

Remove the vendored build tools (pacstrap, mkfs, sgdisk, etc.) that StarForge downloads on first use. These are stored in `~/.local/share/starforge/` and shared across all projects:

```bash
starforge clean deps
```

The dependencies will be re-downloaded automatically on the next build.

## See Also

- [Building & Testing](../building-and-testing/) -- Build commands, caching, and QEMU testing.
- [Installer](../installer/) -- Building self-installing images for device provisioning.
- [`starforge write`](../../commands/write/) -- Write command reference.
- [`starforge export`](../../commands/export/) -- Export command reference.
- [`starforge clean`](../../commands/clean/) -- Clean command reference.
