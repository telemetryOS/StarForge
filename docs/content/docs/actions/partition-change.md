---
title: "partition-change"
weight: 5
---


Modify fields of an existing partition by name. Only the specified fields are changed; unspecified fields keep their current values.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the partition to modify. |
| `filesystem` | string | No | New filesystem type. |
| `size` | string | No | New size (with K/M/G/T suffix, optional `+` for growable). |
| `mount_point` | string | No | New mount point. |
| `type` | string | No | New GPT partition type. |

## Example

```yaml
# Base layer defines a 4G root partition
- action: partition-add
  partitions:
    - name: root
      filesystem: ext4
      size: 4G
      mount_point: /

# Server layer increases root to 16G and makes it growable
- action: partition-change
  name: root
  size: 16G+
```

### Change filesystem

```yaml
- action: partition-change
  name: root
  filesystem: btrfs
```

## Semantics

The named partition is located in the accumulated list and the specified fields are updated. If no partition with the given name exists, the build fails with an error.

## Build Phase

This action modifies the partition list during the Collect stage.

## Notes

- The `name` field identifies which partition to change -- it cannot be used to rename a partition.
- The partition's position in the list is not affected.
- Use [`partition-remove`](partition-remove/) to remove a partition entirely.
