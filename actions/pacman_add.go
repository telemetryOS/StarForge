package actions

import (
	"fmt"
	"strings"

	"github.com/telemetryos/starforge/config"
)

// PkgName extracts the package name before any "=" version pin.
// e.g. "linux=6.12.1-1" → "linux", "base" → "base".
func PkgName(pkg string) string {
	if name, _, ok := strings.Cut(pkg, "="); ok {
		return name
	}
	return pkg
}

// PkgVersion extracts the version after "=" in a package spec.
// Returns "" if no version pin is present.
func PkgVersion(pkg string) string {
	if _, ver, ok := strings.Cut(pkg, "="); ok {
		return ver
	}
	return ""
}

type PacmanAdd struct{}

func (a *PacmanAdd) Name() string { return "pacman-add" }

func (a *PacmanAdd) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.PacmanAdd
	if len(s.Packages) == 0 {
		return fmt.Errorf("pacman-add: packages is required")
	}
	for _, pkg := range s.Packages {
		ctx.Packages = append(ctx.Packages, Package{
			Name:    PkgName(pkg),
			Version: PkgVersion(pkg),
		})
	}
	ctx.PackageGroups = append(ctx.PackageGroups, LayerGroup{
		Layer: ctx.CurrentLayer,
		Items: s.Packages,
	})
	return nil
}
