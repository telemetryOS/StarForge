---
title: "starforge write"
weight: 4
---


Write a target to a device or disk image.

## Usage

```
starforge write <target> <output>
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to write. |
| `output` | Block device path (e.g. `/dev/sdb`) or file path for a compressed disk image (e.g. `./release/device.img.gz`). |

## Description

Writes partition images to a block device or compressed disk image. Creates a GPT partition table and writes each partition image using `bmaptool`. Growable partitions are expanded to fill available space on the device.

The target is built automatically if needed.

## Examples

```bash
# Write to a USB drive
starforge write device /dev/sdb

# Write to an SD card
starforge write device /dev/mmcblk0

# Create a compressed disk image file
starforge write device ./release/device.img.gz
```

## Notes

- **All data on the target device will be destroyed.** A confirmation prompt is shown before writing.
- The device path must start with `/dev/` and must be a block device.
- Requires root access (elevates automatically).
- If the target has installer actions (`install-payload`, `install-server`, `install-client`), installer components are bundled after writing.
- Growable partitions (size ending with `+` in `partition-add`) expand to fill remaining device space.

## See Also

- [build](build/) -- Build disk images for a target
- [export](export/) -- Export as a portable disk image file
