---
title: "install-client"
weight: 52
---


Configure the installer TUI client. The client provides an interactive terminal interface for selecting a payload and target disk, then drives the installation process through the local install-server API.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `auto_login` | string | No | The TTY to configure for autologin. Defaults to `tty1`. |

## Example

```yaml
- action: install-client
  auto_login: tty1
```

## Semantics

**Replace.** If multiple layers configure the installer client, the last layer wins.

## Build Phase

Collected during Collect. Configured during the Package phase, after partition images have been created. The client binary is built from source and copied into the installer's root filesystem.

## Notes

- Installs the `starforge-install` binary to `/usr/bin/starforge-install` inside the target image.
- Configures autologin on the specified TTY by creating a getty drop-in override at `getty@<tty>.service.d/autologin.conf`. This causes root to be logged in automatically on that TTY.
- Creates a `.bash_profile` for root that launches the TUI client on the configured TTY. The check ensures the client only starts on the autologin TTY, not on serial consoles or SSH sessions.
- The TUI client connects to the local install-server API (by default on port 8100).
- See [install-payload](install-payload/) for bundling target images and [install-server](install-server/) for the REST API server.
