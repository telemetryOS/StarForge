# starforge export

Export build artifacts as disk or partition images.

## Usage

```
starforge export <target> <type> [flags]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to export. |
| `type` | Export type: `disk` or `partitions`. |

## Flags

| Flag | Description |
|------|-------------|
| `--size <size>` | Total disk image size (required for `disk` type). Example: `8G`, `16G`. |
| `--output <path>` | Output path: file path for `disk`, directory for `partitions`. |

## Description

### `disk` type

Creates a single bootable GPT disk image file containing all partitions. Growable partitions expand to fill the specified disk size.

```bash
starforge export device disk --size 8G
starforge export device disk --size 16G --output ./release/device.img
```

### `partitions` type

Produces individual partition image files (one per partition). Optionally copies them to an output directory.

```bash
starforge export device partitions
starforge export device partitions --output ./release/
```

## Examples

```bash
# Create an 8G bootable disk image
starforge export device disk --size 8G

# Export to a specific file
starforge export device disk --size 16G --output /tmp/device.img

# Export individual partition images to a release directory
starforge export device partitions --output ./release/
```

## Notes

- Requires a prior `starforge build`.
- Requires root access (elevates automatically).
- For `disk` type, the `--size` flag is required and must be large enough to fit all partitions.
- Without `--output`, disk images are written to `.starforge/<target>/disk.img` and partition images stay in `.starforge/<target>/`.
- The `disk` type is suitable for flashing with `dd` or writing with tools like balenaEtcher.
- The `partitions` type is useful for OTA update systems or custom deployment pipelines.

## See Also

- [build](build.md) -- Build disk images for a target
- [write](write.md) -- Write directly to a physical device
