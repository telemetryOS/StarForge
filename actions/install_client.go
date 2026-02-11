package actions

import (
	"github.com/telemetryos/starforge/config"
)

type InstallClient struct{}

func (a *InstallClient) Name() string { return "install-client" }

func (a *InstallClient) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.InstallClient

	ctx.InstallerClient = &InstallerClientDef{
		AutoLogin: s.AutoLogin,
		Layer:     ctx.CurrentLayer,
	}

	return nil
}
