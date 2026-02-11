package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemdMount struct{}

func (a *SystemdMount) Name() string { return "systemd-mount" }

func (a *SystemdMount) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdMount
	return executeSystemdUnit(
		"systemd-mount", ".mount",
		s.Name, s.User,
		s.Enable, s.Disable, s.Mask,
		s.Extends, s.LayerPath,
		s.UnitSec, s.Mount, "Mount",
		s.Install,
		layerDir, ctx, step.Label,
	)
}
