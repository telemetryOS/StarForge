---
title: "install-server"
weight: 51
---


Configure the installer REST API server. The server provides endpoints for payload listing, disk detection, and installation progress, and is used by the installer TUI client.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `port` | integer | No | Port the server listens on. Defaults to `8100`. |
| `path` | string | No | Directory where payload images are stored inside the target. Must match the directory structure used by `install-payload`. Defaults to `/usr/lib/starforge/payloads`. |
| `efi_label` | string | No | Label for the EFI boot entry created by the installer. |

## Example

```yaml
- action: install-server
  path: /images
```

### With custom port

```yaml
- action: install-server
  port: 9000
  path: /images
```

## Semantics

**Replace.** If multiple layers configure the installer server, the last layer wins.

## Build Phase

Collected during Collect. Configured during the Package phase, after partition images have been created. The server binary is built from source and copied into the installer's root filesystem.

## Notes

- Installs the `starforge-install-server` binary to `/usr/bin/starforge-install-server` inside the target image.
- Creates a systemd service (`starforge-install-server.service`) that starts the server at boot, listening on the configured port and serving payloads from the configured path.
- The service is enabled via a symlink in `multi-user.target.wants`.
- Installer runtime packages (`dosfstools`, `e2fsprogs`, `efibootmgr`, `arch-install-scripts`, `zstd`) are automatically added to the package list.
- See [install-payload](install-payload/) for bundling target images and [install-client](install-client/) for the TUI client.
