---
title: "install-payload"
weight: 50
---


Bundle a built target's partition images into the host target's rootfs. The host can then ship those compressed images for downstream use — the original use case is the installer flashing them onto a device's disk; another is seeding a recovery rootfs with the active boot/root images it can flash to recover from a bad update.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target` | string | Yes | Name of a target defined in the same `starforge.yaml` project. |
| `path` | string | Yes | Directory inside the host rootfs where compressed partition images are stored. For the installer use case, this must be under the `install-server` path so the server can find them at runtime. For a recovery rootfs, this is wherever a recovery script will look for them. |
| `partitions` | list of strings | No | Restrict which of the target's partitions get bundled. Empty (default) bundles all of them. Used to seed a recovery rootfs with just the boot/root images it can flash, without dragging along data/logs/recovery itself. |

## Example

```yaml
- action: install-payload
  target: device
  path: /images/device
  label: Bundle device images
```

### Multiple payloads

```yaml
- action: install-payload
  target: device
  path: /images/device
  label: Production device image

- action: install-payload
  target: kiosk
  path: /images/kiosk
  label: Kiosk device image
```

### Seeding restore images onto recovery partitions

Don't put `install-payload` inside a recovery target that itself is an embed of the host whose images you want — that creates a `recovery → device → recovery` cycle in the build graph. Instead, drop a [post-install lifecycle hook](../install-hooks/) into the installer layer that copies images from the installer USB onto the freshly-flashed recovery partitions. The hook fires after the install pipeline has mounted every target partition; the script just needs `cp` calls.

## Semantics

**Accumulate.** Multiple payloads can be bundled into a single installer image. Each `install-payload` step adds a payload target to the list.

## Build Phase

Collected during Collect. Bundled during the Package phase, after partition images have been created. The payload target's partition images are compressed with zstd and copied into the installer's root filesystem.

## Notes

- The payload target's overlay is built automatically before the host (via the same dependency-resolution pass that handles `install-embed`). When the payload target is also an embed of the host, `install-payload` is treated as a soft dep — no additional recursive build is triggered, and the payload target's already-packaged images are bundled at packaging time.
- Partition images are copied to the specified `path` inside the host's rootfs, alongside a `manifest.json` describing each partition's name, filesystem, size, mount point, type, and image filename. With `partitions:` set, the manifest contains only the listed partitions.
- Images are compressed using `zstd -T0 -9` (multi-threaded, high compression) and stored as `<partition>.img.zst`; a matching `<partition>.img.bmap` sidecar is bundled for sparse, verified copying.
- The `manifest.json` includes a `description` field populated from the step's `label`.
- Installer runtime packages are added by the `install-server` action, not by `install-payload`. The recovery use case typically does NOT include an `install-server` — the recovery rootfs just carries the images and uses its own scripts to flash them.
- See [install-server](install-server/) and [install-client](install-client/) for the other installer actions.
