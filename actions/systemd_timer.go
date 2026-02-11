package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemdTimer struct{}

func (a *SystemdTimer) Name() string { return "systemd-timer" }

func (a *SystemdTimer) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdTimer
	return executeSystemdUnit(
		"systemd-timer", ".timer",
		s.Name, s.User,
		s.Enable, s.Disable, s.Mask,
		s.Extends, s.LayerPath,
		s.UnitSec, s.Timer, "Timer",
		s.Install,
		layerDir, ctx, step.Label,
	)
}
