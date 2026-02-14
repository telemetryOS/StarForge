---
title: "install-payload"
weight: 50
---


Bundle a built target's partition images into the installer image as a payload. This allows the installer to write those images to a device's disk at install time.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target` | string | Yes | Name of a target defined in the same `starforge.yaml` project. |
| `path` | string | Yes | Directory inside the installer image where compressed partition images are stored. Must be under the `install-server` path so the server can find them at runtime. |

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

## Semantics

**Accumulate.** Multiple payloads can be bundled into a single installer image. Each `install-payload` step adds a payload target to the list.

## Build Phase

Collected during Collect. Bundled during the Package phase, after partition images have been created. The payload target's partition images are compressed with zstd and copied into the installer's root filesystem.

## Notes

- The payload target must be built before the installer target. If the payload has not been built, the build will fail with an error directing you to run `starforge build <target>` first.
- Partition images are copied to the specified `path` inside the installer image, alongside a `manifest.json` describing each partition's name, filesystem, size, mount point, type, and image filename.
- Images are compressed using `zstd -T0 -9` (multi-threaded, high compression) and stored as `<partition>.img.zst`.
- The `manifest.json` includes a `description` field populated from the step's `label`.
- Installer runtime packages are added by the `install-server` action, not by `install-payload`. Make sure your installer layer includes an `install-server` step.
- See [install-server](install-server/) and [install-client](install-client/) for the other installer actions.
