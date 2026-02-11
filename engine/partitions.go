package engine

import (
	"github.com/telemetryos/starforge/actions"
)

// ResolvePartitionSizes calculates final sizes for growable partitions.
// If diskSize is 0 or no partitions are growable, partitions keep their defined sizes.
// Otherwise, remaining space after fixed and minimum sizes is divided equally
// among growable partitions.
func ResolvePartitionSizes(partitions []actions.PartitionDef, diskSize uint64) []actions.PartitionDef {
	if diskSize == 0 {
		return partitions
	}

	var growCount int
	var fixedTotal uint64
	for _, p := range partitions {
		fixedTotal += p.Size
		if p.Grow {
			growCount++
		}
	}

	if growCount == 0 || diskSize <= fixedTotal {
		return partitions
	}

	remaining := diskSize - fixedTotal
	perGrow := remaining / uint64(growCount)

	resolved := make([]actions.PartitionDef, len(partitions))
	copy(resolved, partitions)
	for i := range resolved {
		if resolved[i].Grow {
			resolved[i].Size += perGrow
		}
	}

	return resolved
}
