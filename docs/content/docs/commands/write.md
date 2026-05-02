---
title: "starforge write"
weight: 4
---


Write a target to a block device.

## Usage

```
starforge write <target> <device>
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to write. |
| `device` | Block device path (e.g. `/dev/sdb`). |

## Description

Writes partition images to a block device. StarForge creates a GPT partition table and writes each partition image through the Corona writer, which skips redundant image data while explicitly zeroing required ranges on dirty targets. Growable partitions are expanded to fill available space on the device.

The target is built automatically if needed.

## Examples

```bash
# Write to a USB drive
starforge write device /dev/sdb

# Write to an SD card
starforge write device /dev/mmcblk0

# Create a disk image instead of writing hardware
starforge export device disk ./release/device.img
```

## Notes

- **All data on the target device will be destroyed.** A confirmation prompt is shown before writing.
- The device path must start with `/dev/` and must be a block device.
- Requires root access (elevates automatically).
- Disk image files are created with [`starforge export`](../export/), not `starforge write`.
- If the target has installer actions (`install-payload`, `install-server`, `install-client`), installer components are bundled after writing.
- Growable partitions (size ending with `+` in `partition-add`) expand to fill remaining device space.

## See Also

- [build](build/) -- Build disk images for a target
- [export](export/) -- Export as a portable disk image file
