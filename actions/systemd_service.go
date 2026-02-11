package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemdService struct{}

func (a *SystemdService) Name() string { return "systemd-service" }

func (a *SystemdService) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdService
	return executeSystemdUnit(
		"systemd-service", ".service",
		s.Name, s.User,
		s.Enable, s.Disable, s.Mask,
		s.Extends, s.LayerPath,
		s.UnitSec, s.Service, "Service",
		s.Install,
		layerDir, ctx, step.Label,
	)
}
