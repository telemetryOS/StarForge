package actions

import (
	"github.com/telemetryos/starforge/config"
)

type InstallClient struct{}

func (a *InstallClient) Name() string { return "install-client" }

func (a *InstallClient) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.InstallClient

	autoLogin := s.AutoLogin
	if autoLogin == "" {
		autoLogin = "tty1"
	}

	ctx.InstallClient = &InstallClientDef{
		AutoLogin:  autoLogin,
		Unattended: s.Unattended,
		Layer:      ctx.CurrentLayer,
	}

	return nil
}
