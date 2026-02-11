# partition-add

Define disk partitions for the target system. Partitions are created as individual sparse image files during the Package stage and assembled into a GPT disk at write/export/run time.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `partitions` | list of partition objects | Yes | Partition definitions (see below). |
| `after` | string | No | Name of an existing partition to insert after. Without this, partitions are appended. |

### Partition object fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the partition. Used as the image filename (`<name>.img`) and GPT partition label. |
| `filesystem` | string | Yes | Filesystem type: `vfat`, `ext4`, `btrfs`, `xfs`, `swap`, etc. |
| `size` | string | Yes | Partition size with suffix: `K`, `M`, `G`, `T`. Append `+` for growable (expands to fill remaining space on write). |
| `mount_point` | string | Yes | Mount point in the target filesystem (e.g., `/`, `/boot`, `/home`). Use `none` for swap. |
| `type` | string | No | GPT partition type. Default: `linux`. |

### Partition types

| Type | Description |
|------|-------------|
| `linux` | Standard Linux filesystem (default) |
| `efi` | EFI System Partition |
| `bios-boot` | BIOS boot partition |
| `swap` | Linux swap |
| `home` | Linux home |
| `raid` | Linux RAID |
| `lvm` | Linux LVM |
| `microsoft-basic` | Microsoft basic data |
| `microsoft-reserved` | Microsoft reserved |

## Example

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
      size: 8G+
      mount_point: /
```

### Multiple partition-add steps

```yaml
# Base layer defines boot and root
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

# Desktop layer adds a home partition after root
- action: partition-add
  after: root
  partitions:
    - name: home
      filesystem: ext4
      size: 16G+
      mount_point: /home
```

### Growable partitions

Sizes ending with `+` indicate a growable partition. The image is created at the specified minimum size, but when written to a device (`starforge write`) or exported as a disk image (`starforge export disk`), the partition expands to fill any remaining space.

```yaml
- name: root
  filesystem: ext4
  size: 4G+          # At least 4G, grows to fill remaining space
  mount_point: /
```

## Semantics

**Accumulate with replace-on-name.** Partitions from all layers are combined into a single ordered list. If a later layer defines a partition with the same `name` as an earlier one, the later definition replaces the earlier one in place (preserving its position in the list).

## Build Phase

Partition definitions are used during the Package stage (after all 9 execute phases) to create individual sparse image files. The files are then used by `starforge write`, `starforge run`, and `starforge export`.

## Notes

- Partition order matters -- it determines the order on disk.
- The `after` field lets you insert partitions at a specific position relative to existing partitions.
- Use [`partition-remove`](partition-remove.md) to remove a partition and [`partition-change`](partition-change.md) to modify one.
- Each partition becomes a separate `.img` file in the build directory.
- Sparse files are used, so a `size: 8G` partition only uses disk space for the actual data written.
