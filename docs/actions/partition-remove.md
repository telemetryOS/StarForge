# partition-remove

Remove a partition by name from the accumulated partition list.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the partition to remove. |

## Example

```yaml
# Base layer defines boot, root, and home partitions
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
    - name: home
      filesystem: ext4
      size: 16G+
      mount_point: /home

# Minimal layer removes the home partition
- action: partition-remove
  name: home
```

## Semantics

The named partition is removed from the accumulated list. If no partition with the given name exists, the build fails with an error.

## Build Phase

This action modifies the partition list during the Collect stage.

## Notes

- The `name` must match a partition defined by an earlier [`partition-add`](partition-add.md) step.
- Use [`partition-change`](partition-change.md) to modify a partition's fields without removing it.
