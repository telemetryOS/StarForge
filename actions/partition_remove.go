package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type PartitionRemove struct{}

func (a *PartitionRemove) Name() string { return "partition-remove" }

func (a *PartitionRemove) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.PartitionRemove
	if s.Name == "" {
		return fmt.Errorf("partition-remove: name is required")
	}

	idx := -1
	for i, p := range ctx.Partitions {
		if p.Name == s.Name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("partition-remove: no partition named %q", s.Name)
	}

	ctx.Partitions = append(ctx.Partitions[:idx], ctx.Partitions[idx+1:]...)

	ctx.PartitionHistory = append(ctx.PartitionHistory, PartitionSnapshot{
		Layer:      ctx.CurrentLayer,
		Partitions: copyPartitions(ctx.Partitions),
	})

	return nil
}
