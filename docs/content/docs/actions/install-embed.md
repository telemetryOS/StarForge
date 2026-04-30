---
title: "install-embed"
weight: 42
---


Mark another target's full build as a contributor to this target's disk image. The embedded target builds independently — its own rootfs overlay, its own actions, its own packages — and at packaging time its partition declarations are unioned with the host target's by name. The embed's overlay contributes files to whichever partitions it mounts.

This is the mechanism for multi-root systems (A/B, main+recovery) where each rootfs needs its own `/etc`, `/usr`, and systemd configuration but shares a disk and a partition table.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target` | string | Yes | Name of the target to embed, as defined in `starforge.yaml`. |

## Example

```yaml
# Layer for the device target — composes main + recovery on one disk.
- action: install-embed
  target: main

- action: install-embed
  target: recovery
```

The `main` and `recovery` targets each have their own `partition-add` action declaring what they need (including which partition is `/`). The host target's layers (which contain the `install-embed` actions) typically declare disk-level partitions like the ESP that are shared across rootfs trees.

## Partition merge rules

When two or more targets declare a partition with the same name, the partition is deduplicated:

- `filesystem` must match across all declarations (else error)
- `type` must match across all declarations (else error)
- `size` is the maximum across all declarations; if the host declares a size for the partition, the host's size must not be smaller than any embed's (else error)
- `mount_point` is **kept per-target** — the same physical partition can mount at different paths in each rootfs's `fstab`

## File contributions

For each merged partition, every target that mounts it contributes files from its own overlay subtree at its declared mount point.

When more than one target writes to a shared partition, contributions are applied in order — host first, then embeds in post-order over the `install-embed` dependency graph — and **later writers overwrite earlier ones at the same path**. The engine does not arbitrate kernel-file or bootloader-config conflicts; OS layers must be designed so shared paths don't collide. Typical strategies:

- **Use distinct kernel packages**: if a recovery target ships `linux-lts` while the host ships `linux`, their pacman-staged files (`vmlinuz-linux-lts` vs `vmlinuz-linux`, `/lib/modules/<lts-ver>` vs `/lib/modules/<ver>`) don't collide.
- **Choose mount points so pacman writes where the boot entry expects**: the ESP can be mounted at `/efi` in one target and `/boot` in another — same partition, per-target view. Mounting at `/boot` lets pacstrap put kernel files directly on the partition with no extra steps.
- **Only one target should set a `loader:` block** (typically the host). Two targets writing `loader/loader.conf` on a shared boot partition is last-writer-wins and almost always a bug.

## Semantics

**Accumulate.** Multiple `install-embed` actions across layers add to the host's embed list. Embedding the same target name twice is a no-op (deduplicated).

## Build Phase

Recorded during Collect on `BuildContext.InstallEmbeds`. The actual recursive build of embedded targets happens before the host's packaging stage. Each embed produces its own `BuildResult` in `.starforge/<target>/`, just as if it had been built standalone.

## Notes

- The embedded target must be defined in the same `starforge.yaml`. Cycles (a embeds b embeds a) are detected and reported as errors.
- An embedded target can itself contain `install-embed` actions; the build resolves transitively.
- Caching is per-target: changing one embed does not invalidate sibling embeds or the host's own layers.
