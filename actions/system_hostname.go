package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type SystemHostname struct{}

func (a *SystemHostname) Name() string { return "system-hostname" }

func (a *SystemHostname) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemHostname
	if s.Hostname == "" {
		return fmt.Errorf("system-hostname: hostname is required")
	}
	ctx.HostnameHistory = append(ctx.HostnameHistory, LayerValue{
		Layer: ctx.CurrentLayer,
		Value: s.Hostname,
	})
	ctx.Hostname = s.Hostname
	return nil
}
