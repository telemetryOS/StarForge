package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FileOwnership struct{}

func (a *FileOwnership) Name() string { return "file-ownership" }

func (a *FileOwnership) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileOwnership
	if s.Path == "" {
		return fmt.Errorf("file-ownership: path is required")
	}
	if s.Owner == "" && s.Group == "" {
		return fmt.Errorf("file-ownership: owner or group is required")
	}
	ctx.Ownerships = append(ctx.Ownerships, OwnershipOp{
		Path:      s.Path,
		Owner:     s.Owner,
		Group:     s.Group,
		Recursive: s.Recursive,
		Layer:     ctx.CurrentLayer,
		Label:     step.Label,
	})
	return nil
}
