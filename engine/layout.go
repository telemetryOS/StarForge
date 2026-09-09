package engine

import (
	"fmt"
	"strings"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

// ResolveDiskLayout collects a target and its embeds without build side effects.
func ResolveDiskLayout(project *config.Project, targetName string) ([]actions.PartitionDef, error) {
	builder := NewBuilder(project)
	builder.DryRun = true
	contexts := map[string]*actions.BuildContext{}
	state := map[string]int{}
	var embeds []string

	var visit func(string, []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("install-embed cycle: %s", strings.Join(append(path, name), " -> "))
		case 2:
			return nil
		}
		target, ok := project.Targets[name]
		if !ok {
			return fmt.Errorf("target %q not found in project", name)
		}
		state[name] = 1
		ctx, err := builder.Collect(target, false)
		if err != nil {
			return fmt.Errorf("collecting target %q layout: %w", name, err)
		}
		if err := validateTargetHasRoot(name, ctx); err != nil {
			return err
		}
		contexts[name] = ctx
		path = append(path, name)
		for _, embeddedName := range ctx.InstallEmbeds {
			embedded, ok := project.Targets[embeddedName]
			if !ok {
				return fmt.Errorf("install-embed: target %q not found in project", embeddedName)
			}
			if err := validateEmbeddedPlatform(name, target, embeddedName, embedded); err != nil {
				return err
			}
			if err := visit(embeddedName, path); err != nil {
				return err
			}
			if !containsTarget(embeds, embeddedName) {
				embeds = append(embeds, embeddedName)
			}
		}
		state[name] = 2
		return nil
	}

	if err := visit(targetName, nil); err != nil {
		return nil, err
	}
	contributions := []actions.PartitionContribution{{Target: targetName, Parts: contexts[targetName].Partitions}}
	for _, name := range embeds {
		contributions = append(contributions, actions.PartitionContribution{Target: name, Parts: contexts[name].Partitions})
	}
	merged, err := actions.MergePartitions(contributions)
	if err != nil {
		return nil, fmt.Errorf("merging partition tables: %w", err)
	}
	return mergedDiskPartitions(targetName, merged), nil
}

func containsTarget(targets []string, want string) bool {
	for _, target := range targets {
		if target == want {
			return true
		}
	}
	return false
}
