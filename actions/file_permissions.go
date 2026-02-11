package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FilePermissions struct{}

func (a *FilePermissions) Name() string { return "file-permissions" }

func (a *FilePermissions) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FilePermissions
	if s.Path == "" {
		return fmt.Errorf("file-permissions: path is required")
	}
	if s.Mode == "" {
		return fmt.Errorf("file-permissions: mode is required")
	}
	ctx.Permissions = append(ctx.Permissions, PermissionOp{
		Path:      s.Path,
		Mode:      s.Mode,
		Recursive: s.Recursive,
		Layer:     ctx.CurrentLayer,
		Label:     step.Label,
	})
	return nil
}
