package actions

import (
	"strings"
	"testing"
)

func TestRenderUnit_SingleSection(t *testing.T) {
	sections := map[string]map[string]any{
		"Unit": {
			"Description": "Test service",
			"After":       "network.target",
		},
	}

	got := RenderUnit(sections)
	want := "[Unit]\nAfter=network.target\nDescription=Test service\n"
	if got != want {
		t.Errorf("RenderUnit single section:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenderUnit_MultipleSections(t *testing.T) {
	sections := map[string]map[string]any{
		"Service": {
			"Type":      "notify",
			"ExecStart": "/usr/bin/test",
		},
		"Unit": {
			"Description": "Test",
		},
		"Install": {
			"WantedBy": "multi-user.target",
		},
	}

	got := RenderUnit(sections)

	// Sections sorted: Install, Service, Unit
	if !strings.HasPrefix(got, "[Install]\n") {
		t.Error("expected Install section first (alphabetical sort)")
	}
	if !strings.Contains(got, "[Service]\n") {
		t.Error("expected Service section")
	}
	if !strings.Contains(got, "[Unit]\n") {
		t.Error("expected Unit section")
	}

	// Keys within sections are also sorted
	serviceIdx := strings.Index(got, "[Service]")
	svcContent := got[serviceIdx:]
	execIdx := strings.Index(svcContent, "ExecStart=")
	typeIdx := strings.Index(svcContent, "Type=")
	if execIdx > typeIdx {
		t.Error("expected ExecStart before Type in sorted order")
	}
}

func TestRenderUnit_SliceValues(t *testing.T) {
	sections := map[string]map[string]any{
		"Service": {
			"ExecStartPre": []any{
				"-/usr/bin/first",
				"-/usr/bin/second",
			},
		},
	}

	got := RenderUnit(sections)
	if !strings.Contains(got, "ExecStartPre=-/usr/bin/first\n") {
		t.Error("expected first ExecStartPre entry")
	}
	if !strings.Contains(got, "ExecStartPre=-/usr/bin/second\n") {
		t.Error("expected second ExecStartPre entry")
	}
}

func TestRenderUnit_StringSliceValues(t *testing.T) {
	sections := map[string]map[string]any{
		"Service": {
			"ExecStartPre": []string{
				"-/usr/bin/first",
				"-/usr/bin/second",
			},
		},
	}

	got := RenderUnit(sections)
	if !strings.Contains(got, "ExecStartPre=-/usr/bin/first\n") {
		t.Error("expected first ExecStartPre entry")
	}
	if !strings.Contains(got, "ExecStartPre=-/usr/bin/second\n") {
		t.Error("expected second ExecStartPre entry")
	}
}

func TestRenderUnit_EmptySections(t *testing.T) {
	got := RenderUnit(map[string]map[string]any{})
	if got != "" {
		t.Errorf("RenderUnit empty = %q, want empty string", got)
	}
}

// TestRenderUnit_EdgeOSPlayerService tests with realistic data matching
// Edge-OS layers/player/layer.yaml player service configuration.
func TestRenderUnit_EdgeOSPlayerService(t *testing.T) {
	sections := map[string]map[string]any{
		"Unit": {
			"Description": "TelemetryOS Player",
			"After":       "sway-session.target",
			"BindsTo":     "sway-session.target",
		},
		"Service": {
			"Type":           "notify",
			"NotifyAccess":   "all",
			"WatchdogSec":    30,
			"Restart":        "always",
			"RestartSec":     3,
			"ExecStart":      "/home/player/.local/share/player/tos-player --platform node-pro --enable-features=UseOzonePlatform --ozone-platform=wayland",
			"StandardOutput": "journal",
			"StandardError":  "journal",
		},
		"Install": {
			"WantedBy": "sway-session.target",
		},
	}

	got := RenderUnit(sections)

	// Verify all three sections present
	for _, section := range []string{"[Unit]", "[Service]", "[Install]"} {
		if !strings.Contains(got, section) {
			t.Errorf("missing section %s in rendered unit", section)
		}
	}

	// Verify key values from the player service
	expects := []string{
		"Description=TelemetryOS Player",
		"After=sway-session.target",
		"BindsTo=sway-session.target",
		"Type=notify",
		"WatchdogSec=30",
		"Restart=always",
		"WantedBy=sway-session.target",
	}
	for _, exp := range expects {
		if !strings.Contains(got, exp) {
			t.Errorf("missing expected line %q in rendered unit:\n%s", exp, got)
		}
	}
}
