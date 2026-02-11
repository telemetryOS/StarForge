package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type SystemTimezone struct{}

func (a *SystemTimezone) Name() string { return "system-timezone" }

func (a *SystemTimezone) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemTimezone
	if s.Timezone == "" {
		return fmt.Errorf("system-timezone: timezone is required")
	}
	ctx.TimezoneHistory = append(ctx.TimezoneHistory, LayerValue{
		Layer: ctx.CurrentLayer,
		Value: s.Timezone,
	})
	ctx.Timezone = s.Timezone
	return nil
}
