package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FileMove struct{}

func (a *FileMove) Name() string { return "file-move" }

func (a *FileMove) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileMove
	if s.FromPath == "" {
		return fmt.Errorf("file-move: from_path is required")
	}
	if s.ToPath == "" {
		return fmt.Errorf("file-move: to_path is required")
	}
	ctx.Moves = append(ctx.Moves, MoveOp{
		FromPath: s.FromPath,
		ToPath:   s.ToPath,
		Layer:    ctx.CurrentLayer,
		Label:    step.Label,
	})
	return nil
}
