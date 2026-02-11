package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type SystemGroup struct{}

func (a *SystemGroup) Name() string { return "system-group" }

func (a *SystemGroup) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemGroup
	if s.Name == "" {
		return fmt.Errorf("system-group: name is required")
	}

	// Replace existing group with same name
	for i, existing := range ctx.Groups {
		if existing.Name == s.Name {
			ctx.Groups[i] = GroupDef{
				Name:   s.Name,
				GID:    s.GID,
				System: s.System,
				Layer:  ctx.CurrentLayer,
			}
			return nil
		}
	}

	ctx.Groups = append(ctx.Groups, GroupDef{
		Name:   s.Name,
		GID:    s.GID,
		System: s.System,
		Layer:  ctx.CurrentLayer,
	})
	return nil
}
