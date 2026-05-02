---
title: "Corona"
weight: 88
---

Corona is StarForge's chunked image writer and artifact format. It is used directly by `starforge write` when writing raw partition images to block devices, and it can also package a partition image as a `.corona` file when that image needs to be stored, moved, and flashed later without carrying a separate block map file.

The format is designed for installer payloads, recovery payloads, and OTA systems. For normal local development and generic image interchange, raw `.img` exports are still the default.

## Why Corona Exists

Raw partition images are simple, but they are often much larger than the useful data they contain. A filesystem image may be mostly zero-filled free space, and writing it with `dd` spends time copying bytes that do not carry state.

Corona solves three problems:

- **Smaller artifacts.** Zero chunks are represented as zero operations instead of stored as payload bytes.
- **Faster writes.** Flashing writes only useful compressed chunks plus explicit zero ranges.
- **Dirty-target correctness.** Zero ranges are not skipped. They are written back as zeros, so flashing onto an old partition cannot leave stale data behind.

This makes the Corona writer a better fit than plain `dd` for device writes, and makes Corona files a better fit than plain compressed images for device provisioning and recovery. A compressed raw image can transfer fewer bytes, but during restore it still expands back into a full image stream. A Corona file preserves the sparse write plan all the way to the block device.

## Where They Are Used

StarForge uses Corona in three places:

- `starforge write <target> /dev/...` writes raw partition images through the Corona writer.
- `starforge export <target> disk ./disk.corona --format corona` writes a full GPT disk image as a Corona file.
- `install-payload` bundles target partitions as `<partition>.corona` files inside installer or recovery images.
- `starforge export <target> partitions --format corona` writes Corona files to an output directory for release pipelines.

`starforge export <target> partitions` defaults to raw partition `.img` files, and `starforge export <target> disk ./disk.img` defaults to a raw full-disk image. Use `--format corona` only when the consumer understands the Corona format.

The installer server reads each payload manifest, creates the requested partition table, and writes every manifest `artifact` to its matching block partition.

## Architecture

The implementation lives in the `corona` Go package. The package handles both direct image writes and `.corona` artifact reads/writes.

The package has three primary paths:

- **Pack:** `Pack` scans a raw partition image and writes a `.corona` file.
- **Write:** `Write` validates a `.corona` file and applies it to a target path or block device.
- **WriteImage:** writes a raw `.img` directly to a target using the same chunk scheduler and zero-range handling, without first creating a `.corona` file.

StarForge uses `WriteImage` for direct `starforge write` device writes, and uses `Pack`/`Write` for installer payloads and exported Corona files.

## File Layout

A Corona file is a single binary file:

```text
header
chunk frame
chunk frame
...
trailer
```

The header contains:

- 8-byte magic: `CORONA\x00\x02`
- 16-bit format version
- source image size
- chunk size
- filesystem type: unknown, ext, or FAT
- filesystem version
- filesystem block size when known

Each chunk frame records:

- frame type: skip, zero, or zstd
- target offset
- uncompressed size
- compressed size
- CRC32C over the uncompressed chunk for zstd frames
- compressed payload bytes for zstd frames

The trailer contains:

- 8-byte trailer magic: `CFSHA256`
- frame count
- useful byte count
- stored byte count
- SHA-256 of allocated content

The allocated-content SHA-256 validates the meaningful reconstructed data. For supported filesystems, skipped unallocated ranges are intentionally excluded. For unknown or unsupported filesystems, every source range is treated as allocated, so this matches the full source image SHA-256. Per-chunk CRC32C detects decompression or chunk corruption at the exact frame being applied.

## Packing Flow

Packing walks the source image in chunks. The default chunk size is 8 MiB, which gives the worker pool enough independent chunks to keep more CPU cores busy on typical images.

For each chunk or filesystem allocation span:

1. Ask the filesystem allocation checker whether the current range is allocated.
2. If the range is known-unallocated, emit a skip frame without reading or hashing the source bytes.
3. Otherwise, read the range once.
4. Feed the range into the allocated-content hasher.
5. Create a per-frame result channel and pass the range to the compression worker pool.
6. Pass the result channel to the writer in source order.
7. If the range is all zeros, the worker returns a zero frame.
8. Otherwise, the worker computes CRC32C, compresses with zstd, and returns a zstd frame.
9. The writer drains result channels in order and appends frames to the output.
10. After all frames are written, append the trailer.

Compression runs with worker goroutines while the reader computes the source hash. The final artifact remains deterministic because the writer appends frames in source order, regardless of which worker finishes first.

Corona currently understands ext block bitmaps and FAT16/FAT32 allocation tables. FAT12 and any unsupported or suspicious filesystem metadata falls back to full-image packing.

## Flashing Flow

Writing a Corona file starts by scanning the frames, checking bounds, validating per-frame CRC32C, and verifying the reconstructed allocated-content SHA-256. Only then does StarForge open the target for writing.

For each frame:

1. `skip` leaves a proven-unallocated filesystem range untouched.
2. `zero` writes zeros to the target range.
3. `zstd` reads the compressed payload, decompresses it, verifies CRC32C, and writes it at the target offset.

Writes use `WriteAt`, so target writes do not depend on stream position. Frames are validated in source order.

## Safety Properties

Corona writes are designed to be safe for dirty block targets:

- Zero ranges are explicit writes, not assumptions about the target state.
- Frame offsets and sizes are checked against the original image size.
- Out-of-order or overlapping frames are rejected.
- Allocated-content integrity is checked before flashing.
- Each decompressed write chunk is checked before it is written.

Corona does not replace partition-table logic. The caller is still responsible for creating a target partition of the correct size and writing the image or artifact to the correct block device.

## Tradeoffs

Corona is optimized for images with meaningful zero space and for workflows that need reliable progress reporting and direct block writes.

Corona files are not the default export because raw `.img` files are more broadly compatible. A `.corona` file is the better choice when the reader is StarForge, the TelemetryOS updater, or another tool built against the Corona writer.
