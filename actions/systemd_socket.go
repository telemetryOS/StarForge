package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemdSocket struct{}

func (a *SystemdSocket) Name() string { return "systemd-socket" }

func (a *SystemdSocket) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdSocket
	return executeSystemdUnit(
		"systemd-socket", ".socket",
		s.Name, s.User,
		s.Enable, s.Disable, s.Mask,
		s.Extends, s.LayerPath,
		s.UnitSec, s.Socket, "Socket",
		s.Install,
		layerDir, ctx, step.Label,
	)
}
