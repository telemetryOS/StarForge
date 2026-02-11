# TelemetryOS Edge (next)

## Target Hardware
ASUS NUC 14 Essential, Intel N150 (UHD Graphics), 3.6GB RAM, eMMC

## Boot Chain
systemd (multi-user.target) → getty autologin (`--noissue --skip-login`) as `player` → `.profile` → `exec Hyprland` → hyprland.conf exec-once → hyprland-session.target → player.service

## Compositor
Hyprland (Wayland) in kiosk mode. No gaps, borders, animations, or keybindings. All windows auto-fullscreen. XWayland disabled. Cursor hides after 2s inactivity.

Hyprland's `exec-once` propagates Wayland env vars to systemd (`dbus-update-activation-environment` + `systemctl --user import-environment`), then starts `hyprland-session.target` which triggers `player.service`.

Wallpaper via `hyprpaper` — displays `/home/player/.wallpaper-logo.png`.

## Users
- **player** (1001:1001): autologin, no password, passwordless sudo via `/etc/sudoers.d/player`, groups: wheel, video, render, seat, audio, input, data, docker
- **staff** (1002:1003): SSH key auth + password (`fiber-buffer-deploy-vault`), in wheel group (sudo with password)
- **root**: password locked
- **data** user/group (966:1002): owns `/data` partition, mode 2775 (setgid), player in data group

## Seat Management
seatd service enabled. Player in `seat` group. Do NOT disable seatd — logind-only setup caused EGL failures.

## Player-Program
- Source: `~/Developer/TelemetryOS/Player-Program/`
- Build: `pnpm package` (run by user)
- Build output: `out/tos-player-linux-x64/` — copy **entire directory** contents, not just the binary
- Destination in image: `/home/player/.local/share/player/`
- Must be owned 1001:1001

## Deploying Contrib to Image
```bash
sf mount
sudo cp -a target-contrib/edge-next/* mnt/
sudo chown -R 1001:1001 mnt/home/player/
```
`cp -a` as root sets ownership to root — always fix player home ownership after copying.

## Key Config Files
- `etc/systemd/system/getty@tty1.service.d/autologin.conf` — player autologin
- `home/player/.profile` — `exec Hyprland` on tty1
- `home/player/.config/hypr/hyprland.conf` — Hyprland kiosk configuration
- `home/player/.config/hypr/hyprpaper.conf` — wallpaper configuration
- `home/player/.config/systemd/user/player.service` — TelemetryOS Player (Type=notify, WatchdogSec=30)
- `home/player/.config/systemd/user/hyprland-session.target` — session target
- `etc/sudoers.d/player` — NOPASSWD for player
- `etc/ssh/sshd_config.d/10-telemetryos.conf` — key-only SSH auth
- `etc/systemd/system/default.target` — symlink to multi-user.target
- `etc/systemd/logind.conf.d/no-idle.conf` — disable lid switch, idle actions

## Plymouth
bgrt theme (manufacturer logo). `watermark.png` removed from spinner theme. Boot entry needs `quiet splash`.

## Package Dependencies
hyprland, hyprpaper, xdg-desktop-portal-hyprland, seatd
