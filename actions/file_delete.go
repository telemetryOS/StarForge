package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FileDelete struct{}

func (a *FileDelete) Name() string { return "file-delete" }

func (a *FileDelete) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileDelete
	if s.Path == "" {
		return fmt.Errorf("file-delete: path is required")
	}
	ctx.FileDeletes = append(ctx.FileDeletes, FileDeleteOp{
		Path:      s.Path,
		Recursive: s.Recursive,
		Layer:     ctx.CurrentLayer,
		Label:     step.Label,
	})
	return nil
}
