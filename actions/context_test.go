package actions

import (
	"reflect"
	"testing"
)

func TestNewBuildContext_AllSlicesInitialized(t *testing.T) {
	ctx := NewBuildContext()

	// Every slice field must be non-nil so append() and len() work immediately.
	sliceFields := map[string]any{
		"Locales":              ctx.Locales,
		"Packages":             ctx.Packages,
		"PackageGroups":        ctx.PackageGroups,
		"Partitions":           ctx.Partitions,
		"PartitionHistory":     ctx.PartitionHistory,
		"Users":                ctx.Users,
		"Groups":               ctx.Groups,
		"FileMkdirs":           ctx.FileMkdirs,
		"LayerCopies":          ctx.LayerCopies,
		"FileCreates":          ctx.FileCreates,
		"FileEdits":            ctx.FileEdits,
		"FileCopies":           ctx.FileCopies,
		"FileMoves":            ctx.FileMoves,
		"FileLinks":            ctx.FileLinks,
		"FileDeletes":          ctx.FileDeletes,
		"FileOwnerships":       ctx.FileOwnerships,
		"FilePermissions":      ctx.FilePermissions,
		"Scripts":              ctx.Scripts,
		"InstallPayloads":    ctx.InstallPayloads,
		"HostnameHistory":      ctx.HostnameHistory,
		"LocaleHistory":        ctx.LocaleHistory,
		"TimezoneHistory":      ctx.TimezoneHistory,
		"KeymapHistory":        ctx.KeymapHistory,
		"DefaultTargetHistory": ctx.DefaultTargetHistory,
		"EnableGroups":         ctx.EnableGroups,
		"DisableGroups":        ctx.DisableGroups,
		"MaskGroups":           ctx.MaskGroups,
		"UserEnableGroups":     ctx.UserEnableGroups,
		"UserDisableGroups":    ctx.UserDisableGroups,
		"Warnings":             ctx.Warnings,
	}

	for name, field := range sliceFields {
		v := reflect.ValueOf(field)
		if v.Kind() != reflect.Slice {
			t.Errorf("%s is not a slice (got %s)", name, v.Kind())
			continue
		}
		if v.IsNil() {
			t.Errorf("%s is nil — should be initialized to empty slice", name)
		}
	}
}

func TestNewBuildContext_AllMapsInitialized(t *testing.T) {
	ctx := NewBuildContext()

	maps := map[string]any{
		"Vars": ctx.Vars,
		"Env":  ctx.Env,
	}
	for name, field := range maps {
		v := reflect.ValueOf(field)
		if v.Kind() != reflect.Map {
			t.Errorf("%s is not a map", name)
			continue
		}
		if v.IsNil() {
			t.Errorf("%s is nil — should be initialized to empty map", name)
		}
	}
}

func TestNewBuildContext_SlicesAreEmpty(t *testing.T) {
	ctx := NewBuildContext()
	if len(ctx.Packages) != 0 {
		t.Errorf("Packages should be empty, got %d", len(ctx.Packages))
	}
	if len(ctx.FileCreates) != 0 {
		t.Errorf("FileCreates should be empty, got %d", len(ctx.FileCreates))
	}
	if len(ctx.Warnings) != 0 {
		t.Errorf("Warnings should be empty, got %d", len(ctx.Warnings))
	}
}

func TestNewBuildContext_AppendWorksImmediately(t *testing.T) {
	// Verify that append() works on every slice without panicking
	ctx := NewBuildContext()
	ctx.Packages = append(ctx.Packages, Package{Name: "base"})
	ctx.FileCreates = append(ctx.FileCreates, FileCreateOp{Path: "/etc/test"})
	ctx.Warnings = append(ctx.Warnings, "test warning")

	if len(ctx.Packages) != 1 || len(ctx.FileCreates) != 1 || len(ctx.Warnings) != 1 {
		t.Error("append to initialized slices should work")
	}
}

func TestNewBuildContext_DryRunDefaultFalse(t *testing.T) {
	ctx := NewBuildContext()
	if ctx.DryRun {
		t.Error("DryRun should default to false")
	}
}

func TestNewBuildContext_ScalarFieldsZero(t *testing.T) {
	ctx := NewBuildContext()
	if ctx.Hostname != "" {
		t.Errorf("Hostname should be empty, got %q", ctx.Hostname)
	}
	if ctx.Boot != nil {
		t.Error("Boot should be nil")
	}
	if ctx.InstallServer != nil {
		t.Error("InstallServer should be nil")
	}
}
