package actions

import (
	"testing"
)

// All 31 actions that should be registered via init() in actions.go.
var expectedActions = []string{
	// Package management
	"pacman-add", "pacman-remove",
	// File operations
	"file-create", "file-edit", "file-copy", "file-move", "file-delete",
	"file-link", "file-permissions", "file-ownership", "file-mkdir",
	// System configuration
	"system-hostname", "system-locale", "system-timezone", "system-keymap",
	"system-user", "system-group",
	// Systemd units
	"systemd-service", "systemd-mount", "systemd-timer", "systemd-socket",
	"systemd-slice", "systemd-target", "systemd-boot-install",
	// Partitions
	"partition-add", "partition-remove", "partition-change",
	// Scripts
	"run",
	// Installer
	"install-server", "install-client", "install-payload",
}

func TestAllActionsRegistered(t *testing.T) {
	for _, name := range expectedActions {
		a, err := Get(name)
		if err != nil {
			t.Errorf("Get(%q) error: %v", name, err)
			continue
		}
		if a == nil {
			t.Errorf("Get(%q) returned nil", name)
		}
	}
}

func TestActionNameMatchesRegistry(t *testing.T) {
	for _, name := range expectedActions {
		a, err := Get(name)
		if err != nil {
			t.Errorf("Get(%q) error: %v", name, err)
			continue
		}
		if a.Name() != name {
			t.Errorf("Get(%q).Name() = %q, want %q", name, a.Name(), name)
		}
	}
}

func TestGetNonexistent(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Error("Get(\"nonexistent\") expected error")
	}
}

func TestRegistryCount(t *testing.T) {
	// Verify we're testing the right number
	count := len(expectedActions)
	if count != 31 {
		t.Errorf("expected 31 actions in test list, got %d", count)
	}
}
