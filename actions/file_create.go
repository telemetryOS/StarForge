package actions

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/config"
)

type FileCreate struct{}

func (a *FileCreate) Name() string { return "file-create" }

func (a *FileCreate) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileCreate
	if s.Path == "" {
		return fmt.Errorf("file-create: path is required")
	}
	if s.LayerPath != "" && s.Content != "" {
		return fmt.Errorf("file-create: layer_path and content are mutually exclusive")
	}
	if s.LayerPath == "" && s.Content == "" && step.LayerSource == "" {
		return fmt.Errorf("file-create: layer_path, content, or layer_source is required")
	}

	content := s.Content

	if s.LayerPath == "" && s.Content == "" && step.LayerSource != "" {
		// layer_source without layer_path — copy entire source directory
		ctx.Copies = append(ctx.Copies, CopyOp{
			FromPath: ".",
			ToPath:   s.Path,
			LayerDir: layerDir,
			Layer:    ctx.CurrentLayer,
			Label:    step.Label,
		})
		return nil
	}

	if s.LayerPath != "" {
		if config.IsURL(s.LayerPath) {
			// URL — always a single file, read content
			var err error
			content, err = ReadLayerFile(s.LayerPath, layerDir, ctx)
			if err != nil {
				return fmt.Errorf("file-create: %w", err)
			}
		} else {
			srcPath := filepath.Join(layerDir, s.LayerPath)
			info, err := os.Stat(srcPath)
			if err != nil {
				return fmt.Errorf("file-create: stat %s: %w", s.LayerPath, err)
			}

			if info.IsDir() {
				// Directory copy — store as CopyOp
				ctx.Copies = append(ctx.Copies, CopyOp{
					FromPath: s.LayerPath,
					ToPath:   s.Path,
					LayerDir: layerDir,
					Layer:    ctx.CurrentLayer,
					Label:    step.Label,
				})
				return nil
			}

			// Single file — read content at collect time
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("file-create: reading %s: %w", s.LayerPath, err)
			}
			content = string(data)
		}
	}

	mode := s.Mode
	if mode == "" {
		mode = "0644"
	}

	// Replace-on-path: later file-create for same path wins
	for i, fc := range ctx.FileCreates {
		if fc.Path == s.Path {
			ctx.FileCreates[i] = FileCreateOp{
				Path:    s.Path,
				Content: content,
				Mode:    mode,
				Layer:   ctx.CurrentLayer,
				Label:   step.Label,
			}
			return nil
		}
	}

	ctx.FileCreates = append(ctx.FileCreates, FileCreateOp{
		Path:    s.Path,
		Content: content,
		Mode:    mode,
		Layer:   ctx.CurrentLayer,
		Label:   step.Label,
	})
	return nil
}
