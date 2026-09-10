---
title: "starforge run"
weight: 3
---


Boot a target in QEMU for testing.

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

## Description

Verifies the target's sealed full-disk artifact and boots it in a QEMU VM with architecture-specific UEFI firmware and virtio devices. The target VM is a sibling of the builder VM, never a nested guest. QEMU writes to a disposable snapshot, so the sealed artifact is not modified.

The target is built automatically if needed (incremental). Use `starforge build --clean` first to force a full rebuild.

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
- x86-64 or ARM64 UEFI boot
- Virtio disk, network, and GPU
- KVM acceleration for same-architecture targets when available; TCG otherwise
- SSH port forwarding: host 2222 -> guest 22
- Disposable disk snapshot for every run

## Notes

- Requires the QEMU system binary and UEFI firmware for the target architecture.
- Does not require root access, device mapper, loop devices, or host filesystem mounts.
- Runtime changes are discarded when QEMU exits. Rebuild declaratively to preserve changes.

## See Also

- [build](build/) -- Build disk images for a target
- [write](write/) -- Write to physical hardware
