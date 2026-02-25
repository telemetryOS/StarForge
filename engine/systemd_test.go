package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// --- parseInstallSection tests ---

func TestParseInstallSection(t *testing.T) {
	dir := t.TempDir()
	unitContent := `[Unit]
Description=Test Service

[Service]
ExecStart=/usr/bin/test

[Install]
WantedBy=multi-user.target default.target
RequiredBy=critical.target
Alias=mytest.service
Also=helper.service
`
	unitPath := filepath.Join(dir, "test.service")
	os.WriteFile(unitPath, []byte(unitContent), 0o644)

	install, err := parseInstallSection(unitPath)
	if err != nil {
		t.Fatalf("parseInstallSection error: %v", err)
	}

	// WantedBy should have two entries (space-separated)
	if len(install.WantedBy) != 2 {
		t.Fatalf("WantedBy length = %d, want 2", len(install.WantedBy))
	}
	if install.WantedBy[0] != "multi-user.target" || install.WantedBy[1] != "default.target" {
		t.Errorf("WantedBy = %v", install.WantedBy)
	}

	if len(install.RequiredBy) != 1 || install.RequiredBy[0] != "critical.target" {
		t.Errorf("RequiredBy = %v", install.RequiredBy)
	}
	if len(install.Alias) != 1 || install.Alias[0] != "mytest.service" {
		t.Errorf("Alias = %v", install.Alias)
	}
	if len(install.Also) != 1 || install.Also[0] != "helper.service" {
		t.Errorf("Also = %v", install.Also)
	}
}

func TestParseInstallSection_NoInstall(t *testing.T) {
	dir := t.TempDir()
	unitContent := `[Unit]
Description=No Install Section

[Service]
ExecStart=/usr/bin/test
`
	unitPath := filepath.Join(dir, "noinstall.service")
	os.WriteFile(unitPath, []byte(unitContent), 0o644)

	install, err := parseInstallSection(unitPath)
	if err != nil {
		t.Fatalf("parseInstallSection error: %v", err)
	}
	if len(install.WantedBy) != 0 || len(install.RequiredBy) != 0 {
		t.Errorf("expected empty install section, got %+v", install)
	}
}

func TestParseInstallSection_CommentsAndEmpty(t *testing.T) {
	dir := t.TempDir()
	unitContent := `[Install]
# This is a comment
; This is also a comment

WantedBy=multi-user.target
`
	unitPath := filepath.Join(dir, "comments.service")
	os.WriteFile(unitPath, []byte(unitContent), 0o644)

	install, err := parseInstallSection(unitPath)
	if err != nil {
		t.Fatalf("parseInstallSection error: %v", err)
	}
	if len(install.WantedBy) != 1 || install.WantedBy[0] != "multi-user.target" {
		t.Errorf("WantedBy = %v", install.WantedBy)
	}
}

func TestParseInstallSection_SwitchesSections(t *testing.T) {
	dir := t.TempDir()
	unitContent := `[Install]
WantedBy=timers.target

[Unit]
Description=After Install Section
WantedBy=should-not-be-captured
`
	unitPath := filepath.Join(dir, "sections.timer")
	os.WriteFile(unitPath, []byte(unitContent), 0o644)

	install, err := parseInstallSection(unitPath)
	if err != nil {
		t.Fatalf("parseInstallSection error: %v", err)
	}
	// Only the [Install] section WantedBy should be captured
	if len(install.WantedBy) != 1 || install.WantedBy[0] != "timers.target" {
		t.Errorf("WantedBy = %v, want [timers.target]", install.WantedBy)
	}
}

func TestParseInstallSection_EdgeOS_PlayerService(t *testing.T) {
	// Realistic Edge-OS user service
	dir := t.TempDir()
	unitContent := `[Unit]
Description=TelemetryOS Player Application
After=graphical-session.target

[Service]
Type=simple
ExecStart=/opt/player/player
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`
	unitPath := filepath.Join(dir, "player.service")
	os.WriteFile(unitPath, []byte(unitContent), 0o644)

	install, err := parseInstallSection(unitPath)
	if err != nil {
		t.Fatalf("parseInstallSection error: %v", err)
	}
	if len(install.WantedBy) != 1 || install.WantedBy[0] != "default.target" {
		t.Errorf("WantedBy = %v, want [default.target]", install.WantedBy)
	}
}
