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
	if err := validateInstallPath("install-payload", "path", s.Path); err != nil {
		return err
	}

	ctx.InstallPayloads = append(ctx.InstallPayloads, InstallPayloadDef{
		Target:     s.Target,
		Path:       s.Path,
		Partitions: s.Partitions,
		Layer:      ctx.CurrentLayer,
		Label:      step.Label,
	})

	return nil
}
