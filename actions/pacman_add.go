package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type PacmanAdd struct{}

func (a *PacmanAdd) Name() string { return "pacman-add" }

func (a *PacmanAdd) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.PacmanAdd
	if len(s.Packages) == 0 {
		return fmt.Errorf("pacman-add: packages is required")
	}
	ctx.Packages = append(ctx.Packages, s.Packages...)
	ctx.PackageGroups = append(ctx.PackageGroups, LayerGroup{
		Layer: ctx.CurrentLayer,
		Items: s.Packages,
	})
	return nil
}
