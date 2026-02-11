# starforge run

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

Resources are automatically scaled based on the host system:

| Resource | Allocation | Bounds |
|----------|-----------|--------|
| Memory | Half of host RAM | 2 GB -- 16 GB |
| CPUs | Half of host cores | 2 -- 8 |
| GPU memory | Quarter of host RAM | 256 MB -- 2 GB |

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

- [build](build.md) -- Build disk images for a target
- [chroot](chroot.md) -- Enter the filesystem without booting
- [write](write.md) -- Write to physical hardware
