package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type SystemKeymap struct{}

func (a *SystemKeymap) Name() string { return "system-keymap" }

func (a *SystemKeymap) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemKeymap
	if s.Keymap == "" {
		return fmt.Errorf("system-keymap: keymap is required")
	}
	ctx.KeymapHistory = append(ctx.KeymapHistory, LayerValue{
		Layer: ctx.CurrentLayer,
		Value: s.Keymap,
	})
	ctx.Keymap = s.Keymap
	return nil
}
