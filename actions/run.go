package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type Run struct{}

func (a *Run) Name() string { return "run" }

func (a *Run) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.Run

	if s.Script == "" && s.ScriptPath == "" {
		return fmt.Errorf("run: script or script_path is required")
	}
	if s.Script != "" && s.ScriptPath != "" {
		return fmt.Errorf("run: script and script_path are mutually exclusive")
	}

	op := ScriptOp{
		User:     s.User,
		Env:      s.Env,
		LayerDir: layerDir,
		Layer:    ctx.CurrentLayer,
		Label:    step.Label,
	}

	if s.ScriptPath != "" {
		// File-based script: check if it's a URL
		if config.IsURL(s.ScriptPath) {
			content, err := ReadLayerFile(s.ScriptPath, layerDir, ctx)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}
			op.Content = content
		} else {
			op.Script = s.ScriptPath
		}
	} else {
		// Inline script content
		op.Content = s.Script
	}

	ctx.Scripts = append(ctx.Scripts, op)
	return nil
}
