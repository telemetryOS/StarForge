package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// --- parseMode tests ---

func TestParseMode(t *testing.T) {
	tests := []struct {
		input      string
		defaultMod os.FileMode
		want       os.FileMode
	}{
		{"0644", 0o755, 0o644},
		{"0755", 0o644, 0o755},
		{"0600", 0o644, 0o600},
		{"0777", 0o644, 0o777},
		{"0444", 0o644, 0o444},
		{"", 0o644, 0o644},       // empty returns default
		{"invalid", 0o644, 0o644}, // invalid returns default
	}
	for _, tt := range tests {
		got := parseMode(tt.input, tt.defaultMod)
		if got != tt.want {
			t.Errorf("parseMode(%q, %o) = %o, want %o", tt.input, tt.defaultMod, got, tt.want)
		}
	}
}

// --- filesystemForPath tests ---

func TestFilesystemForPath_Default(t *testing.T) {
	// No partitions → default ext4
	got := filesystemForPath("/etc/hostname", nil)
	if got != "ext4" {
		t.Errorf("filesystemForPath with nil parts = %q, want %q", got, "ext4")
	}
}

func TestFilesystemForPath_RootPartition(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
	}
	got := filesystemForPath("/etc/hostname", parts)
	if got != "ext4" {
		t.Errorf("got %q, want %q", got, "ext4")
	}
}

func TestFilesystemForPath_BootPartition(t *testing.T) {
	// Edge-OS layout: boot=vfat, root=ext4
	parts := []actions.PartitionDef{
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
	}
	got := filesystemForPath("/boot/EFI/BOOT/BOOTX64.EFI", parts)
	if got != "vfat" {
		t.Errorf("got %q, want %q", got, "vfat")
	}
}

func TestFilesystemForPath_MostSpecificMatch(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
		{Name: "data", Filesystem: "btrfs", MountPoint: "/var/data"},
	}
	// /var/data/file should match /var/data (more specific than /)
	got := filesystemForPath("/var/data/myfile.db", parts)
	if got != "btrfs" {
		t.Errorf("got %q, want %q", got, "btrfs")
	}

	// /var/log should match / (no /var partition)
	got = filesystemForPath("/var/log/syslog", parts)
	if got != "ext4" {
		t.Errorf("got %q, want %q", got, "ext4")
	}
}

func TestFilesystemForPath_ExactMountPoint(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
	}
	got := filesystemForPath("/boot", parts)
	if got != "vfat" {
		t.Errorf("got %q, want %q", got, "vfat")
	}
}

// --- injectPrelude tests ---

func TestInjectPrelude_WithShebang(t *testing.T) {
	script := "#!/bin/bash\necho hello\n"
	prelude := "# prelude\n"
	got := injectPrelude(script, prelude)
	want := "#!/bin/bash\n# prelude\necho hello\n"
	if got != want {
		t.Errorf("injectPrelude with shebang:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInjectPrelude_WithoutShebang(t *testing.T) {
	script := "echo hello\n"
	prelude := "# prelude\n"
	got := injectPrelude(script, prelude)
	want := "# prelude\necho hello\n"
	if got != want {
		t.Errorf("injectPrelude without shebang:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInjectPrelude_ShebangOnly(t *testing.T) {
	// Shebang with no newline — can't split, so prepend
	script := "#!/bin/bash"
	prelude := "# prelude\n"
	got := injectPrelude(script, prelude)
	want := "# prelude\n#!/bin/bash"
	if got != want {
		t.Errorf("injectPrelude shebang-only:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInjectPrelude_EnvShebang(t *testing.T) {
	script := "#!/usr/bin/env bash\nset -e\necho test\n"
	prelude := "export FOO=bar\n"
	got := injectPrelude(script, prelude)
	want := "#!/usr/bin/env bash\nexport FOO=bar\nset -e\necho test\n"
	if got != want {
		t.Errorf("injectPrelude env shebang:\ngot:  %q\nwant: %q", got, want)
	}
}

// --- mergeScriptEnv tests ---

func TestMergeScriptEnv_BothNil(t *testing.T) {
	got := mergeScriptEnv(nil, nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestMergeScriptEnv_TargetOnly(t *testing.T) {
	target := map[string]string{"FOO": "bar"}
	got := mergeScriptEnv(target, nil)
	if got["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "bar")
	}
}

func TestMergeScriptEnv_StepOverridesTarget(t *testing.T) {
	target := map[string]string{"FOO": "bar", "BAZ": "qux"}
	step := map[string]string{"FOO": "overridden", "NEW": "value"}
	got := mergeScriptEnv(target, step)
	if got["FOO"] != "overridden" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "overridden")
	}
	if got["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", got["BAZ"], "qux")
	}
	if got["NEW"] != "value" {
		t.Errorf("NEW = %q, want %q", got["NEW"], "value")
	}
}

// --- buildScriptPrelude tests ---

func TestBuildScriptPrelude_Empty(t *testing.T) {
	got := buildScriptPrelude(nil)
	if !strings.Contains(got, "sf_set()") {
		t.Error("prelude should contain sf_set")
	}
	if !strings.Contains(got, "sf_get()") {
		t.Error("prelude should contain sf_get")
	}
	if !strings.Contains(got, "declare -A __sf_vars=()") {
		t.Error("prelude should contain empty __sf_vars")
	}
}

func TestBuildScriptPrelude_WithVars(t *testing.T) {
	vars := map[string]string{"hostname": "edge-01", "mode": "production"}
	got := buildScriptPrelude(vars)
	if !strings.Contains(got, "[hostname]='edge-01'") {
		t.Errorf("prelude should contain hostname var, got:\n%s", got)
	}
	if !strings.Contains(got, "[mode]='production'") {
		t.Errorf("prelude should contain mode var, got:\n%s", got)
	}
}

func TestBuildScriptPrelude_SingleQuoteEscaping(t *testing.T) {
	vars := map[string]string{"msg": "it's a test"}
	got := buildScriptPrelude(vars)
	// Single quotes are escaped: ' → '\''
	if !strings.Contains(got, `it'\''s a test`) {
		t.Errorf("single quote not escaped in:\n%s", got)
	}
}

func TestBuildScriptPrelude_SortedKeys(t *testing.T) {
	vars := map[string]string{"z_var": "last", "a_var": "first", "m_var": "middle"}
	got := buildScriptPrelude(vars)
	aIdx := strings.Index(got, "[a_var]")
	mIdx := strings.Index(got, "[m_var]")
	zIdx := strings.Index(got, "[z_var]")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("vars not sorted in prelude:\n%s", got)
	}
}

// --- boolToNo tests ---

func TestBoolToNo(t *testing.T) {
	if boolToNo(true) != "yes" {
		t.Errorf("boolToNo(true) = %q, want %q", boolToNo(true), "yes")
	}
	if boolToNo(false) != "no" {
		t.Errorf("boolToNo(false) = %q, want %q", boolToNo(false), "no")
	}
}

// --- validateImports tests ---

func TestValidateImports_AllPresent(t *testing.T) {
	vars := map[string]string{"foo": "bar", "baz": "qux"}
	err := validateImports([]string{"foo", "baz"}, vars, "test-layer")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateImports_Missing(t *testing.T) {
	vars := map[string]string{"foo": "bar"}
	err := validateImports([]string{"foo", "missing"}, vars, "test-layer")
	if err == nil {
		t.Fatal("expected error for missing import")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention 'missing': %v", err)
	}
}

func TestValidateImports_Empty(t *testing.T) {
	err := validateImports(nil, nil, "test-layer")
	if err != nil {
		t.Errorf("expected no error for nil imports, got: %v", err)
	}
}

// --- substituteString tests ---

func TestSubstituteString_NoVars(t *testing.T) {
	got, err := substituteString("no vars here", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "no vars here" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestSubstituteString_WithVars(t *testing.T) {
	vars := map[string]string{"host": "edge-01", "port": "8080"}
	got, err := substituteString("${{ host }}:${{ port }}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "edge-01:8080" {
		t.Errorf("got %q, want %q", got, "edge-01:8080")
	}
}

func TestSubstituteString_Undefined(t *testing.T) {
	_, err := substituteString("${{ missing }}", map[string]string{})
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
}

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

// --- partitionPath tests ---

func TestPartitionPath(t *testing.T) {
	tests := []struct {
		device string
		num    int
		want   string
	}{
		// Standard SCSI/SATA disks
		{"/dev/sda", 1, "/dev/sda1"},
		{"/dev/sda", 3, "/dev/sda3"},
		{"/dev/sdb", 2, "/dev/sdb2"},
		// NVMe drives (need p separator)
		{"/dev/nvme0n1", 1, "/dev/nvme0n1p1"},
		{"/dev/nvme0n1", 3, "/dev/nvme0n1p3"},
		// Loop devices (need p separator)
		{"/dev/loop0", 1, "/dev/loop0p1"},
		{"/dev/loop5", 2, "/dev/loop5p2"},
		// MMC/SD cards (need p separator)
		{"/dev/mmcblk0", 1, "/dev/mmcblk0p1"},
		{"/dev/mmcblk0", 2, "/dev/mmcblk0p2"},
	}
	for _, tt := range tests {
		got := partitionPath(tt.device, tt.num)
		if got != tt.want {
			t.Errorf("partitionPath(%q, %d) = %q, want %q", tt.device, tt.num, got, tt.want)
		}
	}
}
