package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type InstallPayload struct{}

func (a *InstallPayload) Name() string { return "install-payload" }

func (a *InstallPayload) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.InstallPayload

	if s.Target == "" {
		return fmt.Errorf("install-payload: target is required")
	}
	if s.Path == "" {
		return fmt.Errorf("install-payload: path is required")
	}

	ctx.InstallerPayloads = append(ctx.InstallerPayloads, InstallerPayloadDef{
		Target: s.Target,
		Path:   s.Path,
		Layer:  ctx.CurrentLayer,
		Label:  step.Label,
	})

	return nil
}
