package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type SystemdTarget struct{}

func (a *SystemdTarget) Name() string { return "systemd-target" }

func (a *SystemdTarget) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdTarget

	// Mode 1: Set default target (no unit file involved)
	if s.Target != "" {
		ctx.DefaultTargetHistory = append(ctx.DefaultTargetHistory, LayerValue{
			Layer: ctx.CurrentLayer,
			Value: s.Target,
		})
		ctx.DefaultTarget = s.Target
		return nil
	}

	// Mode 2: Create/manage target unit — delegate to shared systemd unit logic
	if s.Name == "" {
		return fmt.Errorf("systemd-target: target or name is required")
	}

	return executeSystemdUnit(
		"systemd-target",
		".target",
		s.Name, s.User,
		s.Enable, s.Disable, s.Mask,
		nil, // no extends for targets
		s.LayerPath,
		s.UnitSec, nil, // no type-specific section for targets
		"Target",
		s.Install,
		layerDir, ctx, step.Label,
	)
}
