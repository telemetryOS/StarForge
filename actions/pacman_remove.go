package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type PacmanRemove struct{}

func (a *PacmanRemove) Name() string { return "pacman-remove" }

func (a *PacmanRemove) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.PacmanRemove
	if len(s.Packages) == 0 {
		return fmt.Errorf("pacman-remove: packages is required")
	}

	remove := make(map[string]bool, len(s.Packages))
	for _, pkg := range s.Packages {
		remove[PkgName(pkg)] = true
	}

	var filtered []string
	for _, pkg := range ctx.Packages {
		if !remove[PkgName(pkg)] {
			filtered = append(filtered, pkg)
		}
	}
	ctx.Packages = filtered

	return nil
}
