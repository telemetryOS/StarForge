package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemdSlice struct{}

func (a *SystemdSlice) Name() string { return "systemd-slice" }

func (a *SystemdSlice) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdSlice
	return executeSystemdUnit(
		"systemd-slice", ".slice",
		s.Name, s.User,
		s.Enable, s.Disable, s.Mask,
		s.Extends, s.LayerPath,
		s.UnitSec, s.Slice, "Slice",
		s.Install,
		layerDir, ctx, step.Label,
	)
}
