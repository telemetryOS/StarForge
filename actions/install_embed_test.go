package actions

import (
	"testing"

	"github.com/telemetryos/starforge/config"
)

func TestInstallEmbed_RecordsName(t *testing.T) {
	ctx := NewBuildContext()
	execAction(t, config.Step{
		Action:       "install-embed",
		InstallEmbed: &config.InstallEmbedStep{Target: "main"},
	}, ctx)
	execAction(t, config.Step{
		Action:       "install-embed",
		InstallEmbed: &config.InstallEmbedStep{Target: "recovery"},
	}, ctx)
	if len(ctx.InstallEmbeds) != 2 {
		t.Fatalf("InstallEmbeds = %v, want 2", ctx.InstallEmbeds)
	}
	if ctx.InstallEmbeds[0] != "main" || ctx.InstallEmbeds[1] != "recovery" {
		t.Errorf("InstallEmbeds = %v, want [main recovery]", ctx.InstallEmbeds)
	}
}

func TestInstallEmbed_DedupesSameName(t *testing.T) {
	ctx := NewBuildContext()
	execAction(t, config.Step{
		Action:       "install-embed",
		InstallEmbed: &config.InstallEmbedStep{Target: "main"},
	}, ctx)
	execAction(t, config.Step{
		Action:       "install-embed",
		InstallEmbed: &config.InstallEmbedStep{Target: "main"},
	}, ctx)
	if len(ctx.InstallEmbeds) != 1 {
		t.Errorf("InstallEmbeds = %v, want 1 (dedupe)", ctx.InstallEmbeds)
	}
}

func TestInstallEmbed_EmptyTargetErrors(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:       "install-embed",
		InstallEmbed: &config.InstallEmbedStep{Target: ""},
	}, ctx)
	if err == nil {
		t.Error("expected error when target is empty")
	}
}
