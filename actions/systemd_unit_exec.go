package actions

import (
	"fmt"
	"path/filepath"

	"github.com/telemetryos/starforge/config"
)

// executeSystemdUnit implements the common logic for systemd-service, systemd-mount,
// systemd-timer, systemd-socket, and systemd-slice actions.
func executeSystemdUnit(
	actionName string,
	unitExt string,
	name, user string,
	enable, disable, mask bool,
	extends *config.ExtendsRef,
	layerPath string,
	unitSec, typeSec config.UnitSection,
	typeSectionKey string,
	install config.UnitSection,
	layerDir string,
	ctx *BuildContext,
	label string,
) error {
	if name == "" {
		return fmt.Errorf("%s: name is required", actionName)
	}

	unitName := name
	if filepath.Ext(unitName) == "" {
		unitName += unitExt
	}

	// Mask mode
	if mask {
		ctx.Services.Mask = append(ctx.Services.Mask, unitName)
		ctx.MaskGroups = append(ctx.MaskGroups, LayerGroup{
			Layer: ctx.CurrentLayer,
			Items: []string{unitName},
		})
		return nil
	}

	// Enable-only / disable-only mode (no unit content)
	hasContent := layerPath != "" || unitSec != nil || install != nil || typeSec != nil
	if !hasContent {
		if enable {
			addEnable(ctx, user, unitName)
			return nil
		}
		if disable {
			addDisable(ctx, user, unitName)
			return nil
		}
		return fmt.Errorf("%s %s: at least one section, layer_path, enable, disable, or mask is required", actionName, name)
	}

	// Build unit content
	var content string
	if layerPath != "" {
		// Skip download in dry-run (inspect) mode; unit files are never written.
		if !ctx.DryRun {
			var err error
			content, err = ReadLayerFile(layerPath, layerDir, ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", actionName, err)
			}
		}
	} else {
		sections := map[string]map[string]any{}
		if unitSec != nil {
			sections["Unit"] = map[string]any(unitSec)
		}
		if typeSec != nil {
			sections[typeSectionKey] = map[string]any(typeSec)
		}
		if install != nil {
			sections["Install"] = map[string]any(install)
		}
		content = RenderUnit(sections)
	}

	// Determine destination path
	var dest string
	if extends != nil {
		if user != "" {
			return fmt.Errorf("%s: user is not supported with extends (drop-in)", actionName)
		}
		parentUnit := extends.UnitName()
		dest = filepath.Join("/etc/systemd/system", parentUnit+".d", unitName)
	} else if user != "" {
		dest = filepath.Join("/home", user, ".config/systemd/user", unitName)
	} else {
		dest = filepath.Join("/etc/systemd/system", unitName)
	}

	// Write the unit file
	if extends != nil && layerPath != "" && !config.IsURL(layerPath) {
		// Local file with extends: use LayerCopyOp for the drop-in
		ctx.LayerCopies = append(ctx.LayerCopies, LayerCopyOp{
			FromPath: layerPath,
			ToPath:   dest,
			LayerDir: layerDir,
			Layer:    ctx.CurrentLayer,
			Label:    label,
		})
	} else {
		// Replace-on-path for unit files (content already read for URL paths)
		replaced := false
		for i, fc := range ctx.FileCreates {
			if fc.Path == dest {
				ctx.FileCreates[i] = FileCreateOp{
					Path:    dest,
					Content: content,
					Mode:    "0644",
					Layer:   ctx.CurrentLayer,
					Label:   label,
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
				Label:   label,
			})
		}
	}

	if enable {
		addEnable(ctx, user, unitName)
	}
	if disable {
		addDisable(ctx, user, unitName)
	}

	return nil
}

func addEnable(ctx *BuildContext, user, unitName string) {
	if user != "" {
		ctx.Services.UserEnable = append(ctx.Services.UserEnable, UserServiceOp{
			User:    user,
			Service: unitName,
			Layer:   ctx.CurrentLayer,
		})
		ctx.UserEnableGroups = append(ctx.UserEnableGroups, UserServiceGroup{
			Layer: ctx.CurrentLayer,
			User:  user,
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

func addDisable(ctx *BuildContext, user, unitName string) {
	if user != "" {
		ctx.Services.UserDisable = append(ctx.Services.UserDisable, UserServiceOp{
			User:    user,
			Service: unitName,
			Layer:   ctx.CurrentLayer,
		})
		ctx.UserDisableGroups = append(ctx.UserDisableGroups, UserServiceGroup{
			Layer: ctx.CurrentLayer,
			User:  user,
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
