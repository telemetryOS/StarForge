package actions

import (
	"fmt"
	"path/filepath"

	"github.com/telemetryos/starforge/config"
)

type SystemdTarget struct{}

func (a *SystemdTarget) Name() string { return "systemd-target" }

func (a *SystemdTarget) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemdTarget

	// Mode 1: Set default target (existing behavior)
	if s.Target != "" {
		ctx.DefaultTargetHistory = append(ctx.DefaultTargetHistory, LayerValue{
			Layer: ctx.CurrentLayer,
			Value: s.Target,
		})
		ctx.DefaultTarget = s.Target
		return nil
	}

	// Mode 2: Create/manage target file
	if s.Name == "" {
		return fmt.Errorf("systemd-target: target or name is required")
	}

	unitName := s.Name
	if filepath.Ext(unitName) == "" {
		unitName += ".target"
	}

	// Mask mode
	if s.Mask {
		ctx.Services.Mask = append(ctx.Services.Mask, unitName)
		ctx.MaskGroups = append(ctx.MaskGroups, LayerGroup{
			Layer: ctx.CurrentLayer,
			Items: []string{unitName},
		})
		return nil
	}

	// Enable-only / disable-only mode (no unit content)
	hasContent := s.LayerPath != "" || s.UnitSec != nil || s.Install != nil
	if !hasContent {
		if s.Enable {
			if s.User != "" {
				ctx.Services.UserEnable = append(ctx.Services.UserEnable, UserServiceOp{
					User:    s.User,
					Service: unitName,
				})
				ctx.UserEnableGroups = append(ctx.UserEnableGroups, UserServiceGroup{
					Layer: ctx.CurrentLayer,
					User:  s.User,
					Items: []string{unitName},
				})
			} else {
				ctx.Services.Enable = append(ctx.Services.Enable, unitName)
				ctx.EnableGroups = append(ctx.EnableGroups, LayerGroup{
					Layer: ctx.CurrentLayer,
					Items: []string{unitName},
				})
			}
			return nil
		}
		if s.Disable {
			if s.User != "" {
				ctx.Services.UserDisable = append(ctx.Services.UserDisable, UserServiceOp{
					User:    s.User,
					Service: unitName,
				})
				ctx.UserDisableGroups = append(ctx.UserDisableGroups, UserServiceGroup{
					Layer: ctx.CurrentLayer,
					User:  s.User,
					Items: []string{unitName},
				})
			} else {
				ctx.Services.Disable = append(ctx.Services.Disable, unitName)
				ctx.DisableGroups = append(ctx.DisableGroups, LayerGroup{
					Layer: ctx.CurrentLayer,
					Items: []string{unitName},
				})
			}
			return nil
		}
		return fmt.Errorf("systemd-target %s: unit section, layer_path, enable, disable, or mask is required", s.Name)
	}

	// Build the unit content
	var content string
	if s.LayerPath != "" {
		var err error
		content, err = ReadLayerFile(s.LayerPath, layerDir, ctx)
		if err != nil {
			return fmt.Errorf("systemd-target: %w", err)
		}
	} else {
		sections := map[string]map[string]any{}
		if s.UnitSec != nil {
			sections["Unit"] = map[string]any(s.UnitSec)
		}
		if s.Install != nil {
			sections["Install"] = map[string]any(s.Install)
		}
		content = RenderUnit(sections)
	}

	// Determine destination path
	var dest string
	if s.User != "" {
		dest = filepath.Join("/home", s.User, ".config/systemd/user", unitName)
	} else {
		dest = filepath.Join("/etc/systemd/system", unitName)
	}

	// Replace-on-path
	replaced := false
	for i, fc := range ctx.FileCreates {
		if fc.Path == dest {
			ctx.FileCreates[i] = FileCreateOp{
				Path:    dest,
				Content: content,
				Mode:    "0644",
				Layer:   ctx.CurrentLayer,
				Label:   step.Label,
			}
			replaced = true
			break
		}
	}
	if !replaced {
		ctx.FileCreates = append(ctx.FileCreates, FileCreateOp{
			Path:    dest,
			Content: content,
			Mode:    "0644",
			Layer:   ctx.CurrentLayer,
			Label:   step.Label,
		})
	}

	// Handle enable/disable
	if s.Enable {
		if s.User != "" {
			ctx.Services.UserEnable = append(ctx.Services.UserEnable, UserServiceOp{
				User:    s.User,
				Service: unitName,
			})
			ctx.UserEnableGroups = append(ctx.UserEnableGroups, UserServiceGroup{
				Layer: ctx.CurrentLayer,
				User:  s.User,
				Items: []string{unitName},
			})
		} else {
			ctx.Services.Enable = append(ctx.Services.Enable, unitName)
			ctx.EnableGroups = append(ctx.EnableGroups, LayerGroup{
				Layer: ctx.CurrentLayer,
				Items: []string{unitName},
			})
		}
	}
	if s.Disable {
		if s.User != "" {
			ctx.Services.UserDisable = append(ctx.Services.UserDisable, UserServiceOp{
				User:    s.User,
				Service: unitName,
			})
			ctx.UserDisableGroups = append(ctx.UserDisableGroups, UserServiceGroup{
				Layer: ctx.CurrentLayer,
				User:  s.User,
				Items: []string{unitName},
			})
		} else {
			ctx.Services.Disable = append(ctx.Services.Disable, unitName)
			ctx.DisableGroups = append(ctx.DisableGroups, LayerGroup{
				Layer: ctx.CurrentLayer,
				Items: []string{unitName},
			})
		}
	}

	return nil
}
