package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// writeUnitFile creates a minimal systemd unit file with the given [Install] section.
func writeUnitFile(t *testing.T, path, installSection string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	content := "[Unit]\nDescription=Test\n\n" + installSection
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing unit file: %v", err)
	}
}

// --- findUserUnit ---

func TestFindUserUnit_FindsInUserConfigDir(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/myapp.service")
	writeUnitFile(t, unitPath, "[Install]\nWantedBy=default.target\n")

	got := findUserUnit(rootfs, "alice", "myapp.service")
	if got != unitPath {
		t.Errorf("got %q, want %q", got, unitPath)
	}
}

func TestFindUserUnit_FindsInSystemUserDir(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "usr/lib/systemd/user/sysapp.service")
	writeUnitFile(t, unitPath, "[Install]\nWantedBy=timers.target\n")

	got := findUserUnit(rootfs, "alice", "sysapp.service")
	if got != unitPath {
		t.Errorf("got %q, want %q", got, unitPath)
	}
}

func TestFindUserUnit_NotFound_ReturnsEmpty(t *testing.T) {
	rootfs := t.TempDir()
	got := findUserUnit(rootfs, "alice", "nonexistent.service")
	if got != "" {
		t.Errorf("expected empty string for missing unit, got %q", got)
	}
}

func TestFindUserUnit_UserConfigTakesPrecedence(t *testing.T) {
	rootfs := t.TempDir()
	// Same service in both locations
	userPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/app.service")
	sysPath := filepath.Join(rootfs, "usr/lib/systemd/user/app.service")
	writeUnitFile(t, userPath, "[Install]\nWantedBy=default.target\n")
	writeUnitFile(t, sysPath, "[Install]\nWantedBy=timers.target\n")

	got := findUserUnit(rootfs, "alice", "app.service")
	if got != userPath {
		t.Errorf("user config should take precedence: got %q, want %q", got, userPath)
	}
}

// --- enableUserUnit ---

func TestEnableUserUnit_CreatesWantedBySymlink(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/myapp.service")
	writeUnitFile(t, unitPath, "[Install]\nWantedBy=default.target\n")

	if err := enableUserUnit(rootfs, "alice", "myapp.service"); err != nil {
		t.Fatalf("enableUserUnit error: %v", err)
	}

	link := filepath.Join(rootfs, "home/alice/.config/systemd/user/default.target.wants/myapp.service")
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected .wants symlink at %s: %v", link, err)
	}
}

func TestEnableUserUnit_CreatesRequiredBySymlink(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/app.service")
	writeUnitFile(t, unitPath, "[Install]\nRequiredBy=graphical-session.target\n")

	if err := enableUserUnit(rootfs, "alice", "app.service"); err != nil {
		t.Fatalf("enableUserUnit error: %v", err)
	}

	link := filepath.Join(rootfs, "home/alice/.config/systemd/user/graphical-session.target.requires/app.service")
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected .requires symlink at %s: %v", link, err)
	}
}

func TestEnableUserUnit_CreatesAliasSymlink(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/app.service")
	writeUnitFile(t, unitPath, "[Install]\nAlias=myalias.service\n")

	if err := enableUserUnit(rootfs, "alice", "app.service"); err != nil {
		t.Fatalf("enableUserUnit error: %v", err)
	}

	link := filepath.Join(rootfs, "home/alice/.config/systemd/user/myalias.service")
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected alias symlink at %s: %v", link, err)
	}
}

func TestEnableUserUnit_NotFound_ReturnsError(t *testing.T) {
	rootfs := t.TempDir()
	err := enableUserUnit(rootfs, "alice", "nonexistent.service")
	if err == nil {
		t.Fatal("expected error for missing unit")
	}
}

func TestEnableUserUnit_Idempotent(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/app.service")
	writeUnitFile(t, unitPath, "[Install]\nWantedBy=default.target\n")

	enableUserUnit(rootfs, "alice", "app.service")
	if err := enableUserUnit(rootfs, "alice", "app.service"); err != nil {
		t.Fatalf("second enableUserUnit should not error: %v", err)
	}
}

// --- disableUserUnit ---

func TestDisableUserUnit_RemovesWantedBySymlink(t *testing.T) {
	rootfs := t.TempDir()
	unitPath := filepath.Join(rootfs, "home/alice/.config/systemd/user/app.service")
	writeUnitFile(t, unitPath, "[Install]\nWantedBy=default.target\n")

	// Enable first, then disable
	if err := enableUserUnit(rootfs, "alice", "app.service"); err != nil {
		t.Fatalf("setup: enableUserUnit: %v", err)
	}

	if err := disableUserUnit(rootfs, "alice", "app.service"); err != nil {
		t.Fatalf("disableUserUnit error: %v", err)
	}

	link := filepath.Join(rootfs, "home/alice/.config/systemd/user/default.target.wants/app.service")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf(".wants symlink should be removed after disable")
	}
}

func TestDisableUserUnit_NotFound_ReturnsError(t *testing.T) {
	rootfs := t.TempDir()
	err := disableUserUnit(rootfs, "alice", "missing.service")
	if err == nil {
		t.Fatal("expected error for missing unit")
	}
}

// --- phasePreinstall ---

func TestPhasePreinstall_EmptyKeymap_NoOp(t *testing.T) {
	rootfs := t.TempDir()
	o, _ := InitOutput(rootfs, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{}
	b := &Builder{project: nil}
	if err := b.phasePreinstall(ctx, rootfs); err != nil {
		t.Fatalf("phasePreinstall(empty keymap) error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootfs, "etc/vconsole.conf")); !os.IsNotExist(err) {
		t.Error("vconsole.conf should not be created for empty keymap")
	}
}

func TestPhasePreinstall_ValidKeymap_WritesVconsoleConf(t *testing.T) {
	rootfs := t.TempDir()
	o, _ := InitOutput(rootfs, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{Keymap: "us"}
	b := &Builder{project: nil}
	if err := b.phasePreinstall(ctx, rootfs); err != nil {
		t.Fatalf("phasePreinstall error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rootfs, "etc/vconsole.conf"))
	if err != nil {
		t.Fatalf("vconsole.conf not created: %v", err)
	}
	if string(data) != "KEYMAP=us\n" {
		t.Errorf("vconsole.conf = %q, want %q", string(data), "KEYMAP=us\n")
	}
}

func TestPhasePreinstall_CreatesEtcDir(t *testing.T) {
	rootfs := t.TempDir()
	o, _ := InitOutput(rootfs, "test", "target")
	defer o.Close()

	// rootfs has no /etc — phasePreinstall should create it
	ctx := &actions.BuildContext{Keymap: "de"}
	b := &Builder{project: nil}
	if err := b.phasePreinstall(ctx, rootfs); err != nil {
		t.Fatalf("phasePreinstall error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootfs, "etc")); err != nil {
		t.Errorf("/etc not created: %v", err)
	}
}

func TestPhasePreinstall_Idempotent(t *testing.T) {
	rootfs := t.TempDir()
	o, _ := InitOutput(rootfs, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{Keymap: "fr"}
	b := &Builder{project: nil}
	b.phasePreinstall(ctx, rootfs)
	if err := b.phasePreinstall(ctx, rootfs); err != nil {
		t.Fatalf("second phasePreinstall should not error: %v", err)
	}
}
