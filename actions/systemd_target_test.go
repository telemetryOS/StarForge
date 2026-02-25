package actions

import (
	"testing"

	"github.com/telemetryos/starforge/config"
)

func TestSystemdTarget_DefaultTarget(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Target: "multi-user.target",
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.DefaultTarget != "multi-user.target" {
		t.Errorf("DefaultTarget = %q, want %q", ctx.DefaultTarget, "multi-user.target")
	}
	if len(ctx.DefaultTargetHistory) != 1 {
		t.Fatalf("DefaultTargetHistory length = %d, want 1", len(ctx.DefaultTargetHistory))
	}
	if ctx.DefaultTargetHistory[0].Value != "multi-user.target" {
		t.Errorf("history value = %q, want %q", ctx.DefaultTargetHistory[0].Value, "multi-user.target")
	}
	if ctx.DefaultTargetHistory[0].Layer != "test-layer" {
		t.Errorf("history layer = %q, want %q", ctx.DefaultTargetHistory[0].Layer, "test-layer")
	}
}

func TestSystemdTarget_NamedUnit(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		Label:  "custom target",
		SystemdTarget: &config.SystemdTargetStep{
			Name: "my-custom",
			UnitSec: config.UnitSection{
				"Description": "My Custom Target",
			},
			Install: config.UnitSection{
				"WantedBy": "multi-user.target",
			},
			Enable: true,
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create a file
	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates length = %d, want 1", len(ctx.FileCreates))
	}
	fc := ctx.FileCreates[0]
	if fc.Path != "/etc/systemd/system/my-custom.target" {
		t.Errorf("file path = %q, want %q", fc.Path, "/etc/systemd/system/my-custom.target")
	}
	if fc.Mode != "0644" {
		t.Errorf("file mode = %q, want %q", fc.Mode, "0644")
	}

	// Should enable the unit
	if len(ctx.Services.Enable) != 1 {
		t.Fatalf("Enable length = %d, want 1", len(ctx.Services.Enable))
	}
	if ctx.Services.Enable[0] != "my-custom.target" {
		t.Errorf("Enable[0] = %q, want %q", ctx.Services.Enable[0], "my-custom.target")
	}
}

func TestSystemdTarget_EnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Name:   "graphical.target",
			Enable: true,
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No file creation (enable-only)
	if len(ctx.FileCreates) != 0 {
		t.Errorf("FileCreates should be empty for enable-only, got %d", len(ctx.FileCreates))
	}

	// Should enable
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "graphical.target" {
		t.Errorf("Enable = %v, want [graphical.target]", ctx.Services.Enable)
	}
}

func TestSystemdTarget_Mask(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Name: "sleep.target",
			Mask: true,
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Services.Mask) != 1 || ctx.Services.Mask[0] != "sleep.target" {
		t.Errorf("Mask = %v, want [sleep.target]", ctx.Services.Mask)
	}
}

func TestSystemdTarget_DisableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Name:    "emergency.target",
			Disable: true,
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Services.Disable) != 1 || ctx.Services.Disable[0] != "emergency.target" {
		t.Errorf("Disable = %v, want [emergency.target]", ctx.Services.Disable)
	}
}

func TestSystemdTarget_UserEnable(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Name:   "default.target",
			User:   "player",
			Enable: true,
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Services.UserEnable) != 1 {
		t.Fatalf("UserEnable length = %d, want 1", len(ctx.Services.UserEnable))
	}
	if ctx.Services.UserEnable[0].User != "player" {
		t.Errorf("UserEnable user = %q, want %q", ctx.Services.UserEnable[0].User, "player")
	}
	if ctx.Services.UserEnable[0].Service != "default.target" {
		t.Errorf("UserEnable service = %q, want %q", ctx.Services.UserEnable[0].Service, "default.target")
	}
}

func TestSystemdTarget_NoNameOrTarget(t *testing.T) {
	ctx := NewBuildContext()
	step := config.Step{
		Action:        "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{},
	}

	action := &SystemdTarget{}
	err := action.Execute(step, "/tmp", ctx)
	if err == nil {
		t.Fatal("expected error when neither target nor name is set")
	}
}

func TestSystemdTarget_NameWithUserUnit(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test-layer"

	step := config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Name: "app",
			User: "player",
			UnitSec: config.UnitSection{
				"Description": "App Target",
			},
		},
	}

	action := &SystemdTarget{}
	if err := action.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates length = %d, want 1", len(ctx.FileCreates))
	}
	if ctx.FileCreates[0].Path != "/home/player/.config/systemd/user/app.target" {
		t.Errorf("file path = %q, want user path", ctx.FileCreates[0].Path)
	}
}
