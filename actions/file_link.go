package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type FileLink struct{}

func (a *FileLink) Name() string { return "file-link" }

func (a *FileLink) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileLink
	if s.FromPath == "" {
		return fmt.Errorf("file-link: from_path is required")
	}
	if s.ToPath == "" {
		return fmt.Errorf("file-link: to_path is required")
	}
	linkType := s.Type
	if linkType == "" {
		linkType = "symbolic"
	}
	ctx.FileLinks = append(ctx.FileLinks, FileLinkOp{
		FromPath: s.FromPath,
		ToPath:   s.ToPath,
		Type:     linkType,
		Layer:    ctx.CurrentLayer,
		Label:    step.Label,
	})
	return nil
}
