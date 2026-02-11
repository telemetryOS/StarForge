package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

// PartitionChange modifies fields of an existing partition by name.
type PartitionChange struct{}

func (a *PartitionChange) Name() string { return "partition-change" }

func (a *PartitionChange) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.PartitionChange
	if s.Name == "" {
		return fmt.Errorf("partition-change: name is required")
	}

	idx := -1
	for i, p := range ctx.Partitions {
		if p.Name == s.Name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("partition-change: no partition named %q", s.Name)
	}

	if s.Filesystem != "" {
		ctx.Partitions[idx].Filesystem = s.Filesystem
	}
	if s.Size != "" {
		size, grow, err := ParseSize(s.Size)
		if err != nil {
			return fmt.Errorf("partition-change %s: %w", s.Name, err)
		}
		ctx.Partitions[idx].Size = size
		ctx.Partitions[idx].Grow = grow
	}
	if s.MountPoint != "" {
		ctx.Partitions[idx].MountPoint = s.MountPoint
	}
	if s.PartType != "" {
		if !isValidPartitionType(s.PartType) {
			return fmt.Errorf("partition-change %s: unknown partition type %q", s.Name, s.PartType)
		}
		ctx.Partitions[idx].Type = s.PartType
	}

	ctx.PartitionHistory = append(ctx.PartitionHistory, PartitionSnapshot{
		Layer:      ctx.CurrentLayer,
		Partitions: copyPartitions(ctx.Partitions),
	})

	return nil
}
