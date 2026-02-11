# TelemetryOS Edge

## Target Hardware
ASUS NUC 14 Essential, Intel N150 (UHD Graphics), 3.6GB RAM, eMMC

## Boot Chain
systemd (multi-user.target) → getty autologin (`--noissue --skip-login`) as `player` → `.profile` → `exec sway` → sway-systemd session.sh → player.service

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
sudo cp -a target-contrib/edge/* mnt/
sudo chown -R 1001:1001 mnt/home/player/
```
`cp -a` as root sets ownership to root — always fix player home ownership after copying.

## Key Config Files
- `etc/systemd/system/getty@tty1.service.d/autologin.conf` — player autologin
- `home/player/.profile` — `exec sway` on tty1
- `home/player/.config/systemd/user/player.service` — TelemetryOS Player (Type=notify, WatchdogSec=30)
- `etc/sudoers.d/player` — NOPASSWD for player
- `etc/ssh/sshd_config.d/10-telemetryos.conf` — key-only SSH auth
- `etc/systemd/system/default.target` — symlink to multi-user.target
- `etc/systemd/logind.conf.d/no-idle.conf` — disable lid switch, idle actions

## Plymouth
bgrt theme (manufacturer logo). `watermark.png` removed from spinner theme. Boot entry needs `quiet splash`.

## sway-systemd Dependencies
python-dbus-fast, python-i3ipc, python-psutil, python-tenacity, python-xlib
