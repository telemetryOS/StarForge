package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemdBootInstall struct{}

func (a *SystemdBootInstall) Name() string { return "systemd-boot-install" }

func (a *SystemdBootInstall) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdBootInstall

	if ctx.Boot == nil {
		ctx.Boot = &BootConfig{}
	}

	ctx.Boot.Layer = ctx.CurrentLayer

	if s.Loader != nil {
		ctx.Boot.Loader = *s.Loader
	}

	if len(s.Entries) > 0 {
		ctx.Boot.Entries = append(ctx.Boot.Entries, s.Entries...)
	}

	return nil
}
