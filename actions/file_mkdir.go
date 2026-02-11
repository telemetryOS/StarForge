package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FileMkdir struct{}

func (a *FileMkdir) Name() string { return "file-mkdir" }

func (a *FileMkdir) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileMkdir
	if s.Path == "" {
		return fmt.Errorf("file-mkdir: path is required")
	}
	ctx.Mkdirs = append(ctx.Mkdirs, MkdirOp{
		Path:  s.Path,
		Owner: s.Owner,
		Group: s.Group,
		Mode:  s.Mode,
		Layer: ctx.CurrentLayer,
		Label: step.Label,
	})
	return nil
}
