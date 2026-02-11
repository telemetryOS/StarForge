package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type PartitionAdd struct{}

func (a *PartitionAdd) Name() string { return "partition-add" }

func (a *PartitionAdd) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.PartitionAdd
	if len(s.Partitions) == 0 {
		return fmt.Errorf("partition-add: partitions is required")
	}

	var newParts []PartitionDef
	for _, p := range s.Partitions {
		if p.Name == "" {
			return fmt.Errorf("partition-add: partition name is required")
		}
		if p.Filesystem == "" {
			return fmt.Errorf("partition-add %s: filesystem is required", p.Name)
		}
		if p.Size == "" {
			return fmt.Errorf("partition-add %s: size is required", p.Name)
		}

		size, grow, err := ParseSize(p.Size)
		if err != nil {
			return fmt.Errorf("partition-add %s: %w", p.Name, err)
		}

		partType := p.Type
		if partType == "" {
			partType = "linux"
		}
		if !isValidPartitionType(partType) {
			return fmt.Errorf("partition-add %s: unknown partition type %q", p.Name, partType)
		}

		newParts = append(newParts, PartitionDef{
			Name:       p.Name,
			Filesystem: p.Filesystem,
			Size:       size,
			MountPoint: p.MountPoint,
			Type:       partType,
			Grow:       grow,
		})
	}

	if s.After != "" {
		// Insert after the named partition
		idx := -1
		for i, p := range ctx.Partitions {
			if p.Name == s.After {
				idx = i
				break
			}
		}
		if idx == -1 {
			return fmt.Errorf("partition-add: no partition named %q to insert after", s.After)
		}
		// Insert after idx
		tail := make([]PartitionDef, len(ctx.Partitions[idx+1:]))
		copy(tail, ctx.Partitions[idx+1:])
		ctx.Partitions = append(ctx.Partitions[:idx+1], newParts...)
		ctx.Partitions = append(ctx.Partitions, tail...)
	} else {
		ctx.Partitions = append(ctx.Partitions, newParts...)
	}

	ctx.PartitionHistory = append(ctx.PartitionHistory, PartitionSnapshot{
		Layer:      ctx.CurrentLayer,
		Partitions: copyPartitions(ctx.Partitions),
	})

	return nil
}
