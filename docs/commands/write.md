# starforge write

Write a built target to a storage device.

## Usage

```
starforge write <target> <device>
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to write. |
| `device` | Block device path (e.g., `/dev/sdb`, `/dev/mmcblk0`). |

## Description

Writes the pre-built partition images from the last build directly to a block device. Creates a GPT partition table and writes each partition image using `dd`. Growable partitions are expanded to fill available space on the device.

Requires a prior `starforge build`.

## Example

```bash
# Write to a USB drive
starforge write device /dev/sdb

# Write to an SD card
starforge write device /dev/mmcblk0
```

## Notes

- **All data on the target device will be destroyed.** A confirmation prompt is shown before writing.
- The device path must start with `/dev/` and must be a block device.
- Requires root access (elevates automatically).
- If the target has installer actions (`install-payload`, `install-server`, `install-client`), installer components are bundled after writing.
- Growable partitions (size ending with `+` in `partition-add`) expand to fill remaining device space.

## See Also

- [build](build.md) -- Build disk images for a target
- [export](export.md) -- Export as a portable disk image file
