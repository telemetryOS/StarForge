---
title: "starforge export"
weight: 6
---


Export build artifacts as disk images, partition images, or Corona files.

## Usage

```
starforge export <target> <type> [output] [flags]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to export. |
| `type` | Export type: `disk` or `partitions`. |
| `output` | Optional destination path. For `disk`, this is a file path. For `partitions`, this is a directory path. |

## Flags

| Flag | Description |
|------|-------------|
| `--format <format>` | Export format: `image` (default) or `corona`. |

## Description

### `disk` type

Creates a single bootable GPT disk image file containing all partitions. The image size is derived from the target's partition layout.

```bash
starforge export device disk
starforge export device disk ./release/device.img
starforge export device disk ./release/device.corona --format corona
```

### `partitions` type

Produces individual partition image files (one per partition). Optionally copies them to an output directory. Use `--format corona` to export each partition as a Corona file instead of a raw `.img`.

```bash
starforge export device partitions
starforge export device partitions ./release/
starforge export device partitions ./release/ --format corona
```

## Examples

```bash
# Create a bootable disk image
starforge export device disk

# Export to a specific file
starforge export device disk /tmp/device.img

# Export a full-disk Corona file
starforge export device disk /tmp/device.corona --format corona

# Export individual partition images to a release directory
starforge export device partitions ./release/

# Export individual partition Corona files to a release directory
starforge export device partitions ./release/ --format corona
```

## Notes

- The target is built automatically if needed.
- Requires root access (elevates automatically).
- Without an output argument, disk images are written to `.starforge/<target>/disk.img` or `.starforge/<target>/disk.corona`, and partition images stay in `.starforge/<target>/`.
- The default `disk` image format is suitable for flashing with `dd` or disk imaging tools.
- The `partitions` type is useful for OTA update systems or custom deployment pipelines.

## See Also

- [build](build/) -- Build disk images for a target
- [write](write/) -- Write directly to a physical device
