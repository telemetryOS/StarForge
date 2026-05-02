---
title: "Partitions"
weight: 3
---

StarForge defines disk layout declaratively using the `partition-add`, `partition-remove`, and `partition-change` actions. Partitions accumulate across layers, allowing a base layer to establish the layout while later layers extend or modify it.

## Defining Partitions with `partition-add`

The `partition-add` action declares one or more partitions. Each partition requires a name, filesystem type, size, and mount point:

```yaml
- action: partition-add
  partitions:
    - name: boot
      filesystem: vfat
      size: 512M
      mount_point: /boot
      type: efi
    - name: root
      filesystem: ext4
      size: 8G
      mount_point: /
      type: linux
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique partition identifier. Used for cross-layer references. |
| `filesystem` | string | Yes | Filesystem type: `ext4`, `vfat`, `btrfs`, `xfs`, `swap`, etc. |
| `size` | string | Yes | Partition size with suffix. See [Size Syntax](#size-syntax) below. |
| `mount_point` | string | Yes | Absolute path where the partition is mounted in the target filesystem. |
| `type` | string | No | GPT partition type. Defaults to `linux`. See [Partition Types](#partition-types) below. |

## Size Syntax

Partition sizes use integer values with a unit suffix:

| Suffix | Unit |
|--------|------|
| `K` | Kilobytes |
| `M` | Megabytes |
| `G` | Gigabytes |
| `T` | Terabytes |

### Growable Partitions

Append `+` to a size to make the partition growable. A growable partition has a minimum size for the build image and expands to fill remaining disk space when writing to a real device. Disk exports use the target's natural image size unless the layout declares larger fixed partition sizes.

```yaml
- name: data
  filesystem: ext4
  size: 256M+        # At least 256M, grows to fill remaining space
  mount_point: /data
  type: linux
```

Use `100%` to indicate a partition that fills all remaining space with no minimum size requirement:

```yaml
- name: storage
  filesystem: ext4
  size: 100%
  mount_point: /storage
  type: linux
```

Only one partition should use growable sizing. If multiple partitions are marked as growable, the last one in the final partition list receives the remaining space.

## Partition Types

The `type` field maps to GPT partition type codes. If omitted, it defaults to `linux`.

| Type | Description |
|------|-------------|
| `efi` | EFI System Partition (required for UEFI boot) |
| `bios-boot` | BIOS boot partition (for legacy BIOS booting) |
| `linux` | Linux filesystem (default) |
| `home` | Linux home partition |
| `swap` | Linux swap |
| `raid` | Linux RAID |
| `lvm` | Linux LVM |
| `microsoft-basic` | Microsoft basic data |
| `microsoft-reserved` | Microsoft reserved |

## Accumulation Across Layers

Partitions accumulate across layers. A later layer can add more partitions without redeclaring the entire layout:

```yaml
# Base layer -- defines core partitions
- action: partition-add
  partitions:
    - name: boot
      filesystem: vfat
      size: 1G
      mount_point: /boot
      type: efi
    - name: root
      filesystem: ext4
      size: 12G
      mount_point: /
      type: linux

# Feature layer -- adds a logs partition
- action: partition-add
  partitions:
    - name: logs
      filesystem: ext4
      size: 512M
      mount_point: /var/log
      type: linux
```

The resulting layout contains all three partitions in the order they were defined.

To modify an existing partition's fields without adding a duplicate, use `partition-change` instead:

```yaml
# Later layer -- increase root size without re-adding
- action: partition-change
  name: root
  size: 16G
```

### Insertion Ordering with `after`

The `after` field controls where new partitions are inserted relative to existing ones:

```yaml
# Insert a recovery partition after root
- action: partition-add
  after: root
  partitions:
    - name: recovery
      filesystem: ext4
      size: 4G
      mount_point: /recovery
      type: linux
```

Without `after`, new partitions are appended to the end of the list.

## Removing Partitions with `partition-remove`

The `partition-remove` action removes a partition by name:

```yaml
- action: partition-remove
  name: recovery
```

This is useful when a later layer needs to remove a partition defined by an earlier layer. If the named partition does not exist, the build fails with an error.

## Modifying Partitions with `partition-change`

The `partition-change` action modifies fields of an existing partition without replacing it entirely. Only the specified fields are updated; all other fields retain their current values:

```yaml
# Increase the root partition size
- action: partition-change
  name: root
  size: 16G

# Change a mount point
- action: partition-change
  name: data
  mount_point: /storage
```

This is more precise than re-adding the partition with `partition-add`, because `partition-change` preserves any fields you do not explicitly set.

## Real-World Example

This example defines a six-partition layout in a base layer, including growable data storage and dual recovery partitions:

```yaml
- action: partition-add
  label: Disk layout (boot, root, recovery, logs, data)
  partitions:
    - name: boot
      filesystem: vfat
      size: 1G
      mount_point: /boot
      type: efi
    - name: root
      filesystem: ext4
      size: 12G
      mount_point: /
      type: linux
    - name: fallback-recovery
      filesystem: ext4
      size: 6G
      mount_point: /fallback-recovery
      type: linux
    - name: recovery
      filesystem: ext4
      size: 6G
      mount_point: /recovery
      type: linux
    - name: logs
      filesystem: ext4
      size: 512M
      mount_point: /var/log
      type: linux
    - name: data
      filesystem: ext4
      size: 256M+
      mount_point: /data
      type: linux
```

The `data` partition uses `256M+` so it has at least 256 MB during the build but expands to consume all remaining disk space when written to a physical device. This pattern is common for partitions that store variable-sized application data.

## See Also

- [`partition-add`](../../actions/partition-add/) -- Full action reference.
- [`partition-remove`](../../actions/partition-remove/) -- Full action reference.
- [`partition-change`](../../actions/partition-change/) -- Full action reference.
- [Writing Layers](../writing-layers/) -- Override semantics for partitions and other actions.
- [Building & Testing](../building-and-testing/) -- Inspecting the resolved partition layout with `starforge inspect`.
