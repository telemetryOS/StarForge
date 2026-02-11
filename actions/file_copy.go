package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FileCopy struct{}

func (a *FileCopy) Name() string { return "file-copy" }

func (a *FileCopy) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileCopy
	if s.FromPath == "" {
		return fmt.Errorf("file-copy: from_path is required")
	}
	if s.ToPath == "" {
		return fmt.Errorf("file-copy: to_path is required")
	}
	ctx.FileCopies = append(ctx.FileCopies, FileCopyOp{
		FromPath: s.FromPath,
		ToPath:   s.ToPath,
		Layer:    ctx.CurrentLayer,
		Label:    step.Label,
	})
	return nil
}
