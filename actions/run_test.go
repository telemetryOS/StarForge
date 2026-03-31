package actions

import (
	"testing"

	"github.com/telemetryos/starforge/config"
)

func TestRun_DryRun_URLScript_NoDownload(t *testing.T) {
	// In DryRun mode a URL script_path must not trigger a download.
	// Execute must succeed and record a ScriptOp with empty Content.
	ctx := NewBuildContext()
	ctx.DryRun = true

	step := config.Step{
		Action: "run",
		Run: &config.RunStep{
			ScriptPath: "https://example.invalid/setup.sh",
		},
	}

	a := &Run{}
	if err := a.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("Execute in DryRun mode returned error: %v", err)
	}

	if len(ctx.Scripts) != 1 {
		t.Fatalf("expected 1 ScriptOp, got %d", len(ctx.Scripts))
	}
	if ctx.Scripts[0].Content != "" {
		t.Errorf("DryRun: Content should be empty, got %q", ctx.Scripts[0].Content)
	}
}

func TestRun_DryRun_InlineScript_Recorded(t *testing.T) {
	// Inline scripts are always recorded regardless of DryRun.
	ctx := NewBuildContext()
	ctx.DryRun = true

	step := config.Step{
		Action: "run",
		Run: &config.RunStep{
			Script: "echo hello",
		},
	}

	a := &Run{}
	if err := a.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(ctx.Scripts) != 1 {
		t.Fatalf("expected 1 ScriptOp, got %d", len(ctx.Scripts))
	}
	if ctx.Scripts[0].Content != "echo hello" {
		t.Errorf("Content = %q, want %q", ctx.Scripts[0].Content, "echo hello")
	}
}

func TestRun_LocalScriptPath_NotAffectedByDryRun(t *testing.T) {
	// Local script paths are stored by path (not read), so DryRun doesn't change behaviour.
	ctx := NewBuildContext()
	ctx.DryRun = true

	step := config.Step{
		Action: "run",
		Run: &config.RunStep{
			ScriptPath: "scripts/setup.sh",
		},
	}

	a := &Run{}
	if err := a.Execute(step, "/tmp", ctx); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(ctx.Scripts) != 1 {
		t.Fatalf("expected 1 ScriptOp, got %d", len(ctx.Scripts))
	}
	if ctx.Scripts[0].Script != "scripts/setup.sh" {
		t.Errorf("Script = %q, want %q", ctx.Scripts[0].Script, "scripts/setup.sh")
	}
	if ctx.Scripts[0].Content != "" {
		t.Errorf("Content should be empty for local path, got %q", ctx.Scripts[0].Content)
	}
}

func TestRun_MissingScriptError(t *testing.T) {
	ctx := NewBuildContext()
	step := config.Step{
		Action: "run",
		Run:    &config.RunStep{},
	}
	if err := (&Run{}).Execute(step, "/tmp", ctx); err == nil {
		t.Fatal("expected error for missing script/script_path")
	}
}

func TestRun_BothScriptAndPathError(t *testing.T) {
	ctx := NewBuildContext()
	step := config.Step{
		Action: "run",
		Run: &config.RunStep{
			Script:     "echo hi",
			ScriptPath: "setup.sh",
		},
	}
	if err := (&Run{}).Execute(step, "/tmp", ctx); err == nil {
		t.Fatal("expected error for both script and script_path set")
	}
}
