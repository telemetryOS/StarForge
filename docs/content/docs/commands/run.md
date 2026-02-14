---
title: "starforge run"
weight: 3
---


Boot a built target in QEMU for testing.

## Usage

```
starforge run [flags] <target>
```

## Arguments

| Argument | Description |
|----------|-------------|
| `target` | Name of the target to boot. |

## Flags

| Flag | Description |
|------|-------------|
| `--serial` | Attach the serial console to the terminal (instead of opening a QEMU window). |
| `--overlay <name>` | Use a named overlay for persistent changes across sessions. |
| `--boot-disk <name>` | Boot from a named QEMU disk instead of the build target. |

## Description

Assembles partition images into a virtual disk via device mapper and boots in QEMU with OVMF UEFI firmware and virtio devices.

Requires a prior `starforge build`.

SSH is forwarded on port 2222:

```bash
ssh -p 2222 localhost
```

## Examples

```bash
# Boot with QEMU window
starforge run device

# Boot with serial console in terminal
starforge run --serial device

# Boot with persistent overlay
starforge run --overlay testing device
```

## QEMU Configuration

Default resource allocation:

| Resource | Default |
|----------|---------|
| Memory | 4 GB |
| CPUs | 4 |
| GPU memory | 512 MB |

These can be overridden in the target's `qemu` configuration in `starforge.yaml`.

Features:
- UEFI boot via OVMF firmware
- Virtio disk, network, and GPU
- KVM acceleration (if available)
- SSH port forwarding: host 2222 -> guest 22

## Notes

- Requires QEMU (`qemu-system-x86_64`) to be installed on the host. This is the only tool that must be manually installed.
- Requires root access for device mapper and loop device setup.
- Stale device mapper and loop devices from previous crashed runs are cleaned up automatically.
- The virtual disk is assembled from individual partition images, not a single disk image.
- OVMF firmware, dmsetup, and other run dependencies are vendored automatically.

## See Also

- [build](build/) -- Build disk images for a target
- [chroot](chroot/) -- Enter the filesystem without booting
- [write](write/) -- Write to physical hardware
