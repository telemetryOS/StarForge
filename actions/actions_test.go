package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/config"
)

// helper to create a step and execute an action
func execAction(t *testing.T, step config.Step, ctx *BuildContext) {
	t.Helper()
	a, err := Get(step.Action)
	if err != nil {
		t.Fatalf("Get(%q): %v", step.Action, err)
	}
	if err := a.Execute(step, "/tmp/test-layer", ctx); err != nil {
		t.Fatalf("%s.Execute: %v", step.Action, err)
	}
}

func execActionErr(t *testing.T, step config.Step, ctx *BuildContext) error {
	t.Helper()
	a, err := Get(step.Action)
	if err != nil {
		t.Fatalf("Get(%q): %v", step.Action, err)
	}
	return a.Execute(step, "/tmp/test-layer", ctx)
}

// --- Packages ---

func TestPacmanAdd_Accumulates(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:   "pacman-add",
		PacmanAdd: &config.PacmanAddStep{Packages: []string{"base", "linux"}},
	}, ctx)
	execAction(t, config.Step{
		Action:   "pacman-add",
		PacmanAdd: &config.PacmanAddStep{Packages: []string{"sudo"}},
	}, ctx)
	if len(ctx.Packages) != 3 {
		t.Errorf("Packages = %v, want 3 items", ctx.Packages)
	}
	if len(ctx.PackageGroups) != 2 {
		t.Errorf("PackageGroups = %d, want 2", len(ctx.PackageGroups))
	}
}

func TestPacmanAdd_EmptyError(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:   "pacman-add",
		PacmanAdd: &config.PacmanAddStep{Packages: []string{}},
	}, ctx)
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

func TestPacmanRemove_Filters(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	ctx.Packages = []string{"base", "linux", "sudo", "nano"}
	execAction(t, config.Step{
		Action:      "pacman-remove",
		PacmanRemove: &config.PacmanRemoveStep{Packages: []string{"nano", "sudo"}},
	}, ctx)
	if len(ctx.Packages) != 2 {
		t.Errorf("Packages after remove = %v", ctx.Packages)
	}
}

func TestPacmanRemove_EmptyError(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:      "pacman-remove",
		PacmanRemove: &config.PacmanRemoveStep{Packages: []string{}},
	}, ctx)
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

// --- Partitions ---

func TestPartitionAdd_Basic(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "partition-add",
		PartitionAdd: &config.PartitionAddStep{
			Partitions: []config.Partition{
				{Name: "boot", Filesystem: "vfat", Size: "1G", MountPoint: "/boot", Type: "efi"},
				{Name: "root", Filesystem: "ext4", Size: "12G", MountPoint: "/"},
			},
		},
	}, ctx)
	if len(ctx.Partitions) != 2 {
		t.Fatalf("Partitions = %d, want 2", len(ctx.Partitions))
	}
	if ctx.Partitions[0].Name != "boot" || ctx.Partitions[0].Type != "efi" {
		t.Errorf("boot partition = %+v", ctx.Partitions[0])
	}
	if ctx.Partitions[1].Type != "linux" {
		t.Errorf("root partition type = %q, want 'linux' (default)", ctx.Partitions[1].Type)
	}
	if ctx.Partitions[0].Size != 1<<30 {
		t.Errorf("boot size = %d, want %d", ctx.Partitions[0].Size, uint64(1<<30))
	}
}

func TestPartitionAdd_Growable(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS data partition: 256M+ (growable with minimum)
	execAction(t, config.Step{
		Action: "partition-add",
		PartitionAdd: &config.PartitionAddStep{
			Partitions: []config.Partition{
				{Name: "data", Filesystem: "ext4", Size: "256M+", MountPoint: "/data"},
			},
		},
	}, ctx)
	if !ctx.Partitions[0].Grow {
		t.Error("data partition should be growable")
	}
	if ctx.Partitions[0].Size != 256<<20 {
		t.Errorf("data size = %d, want %d", ctx.Partitions[0].Size, uint64(256<<20))
	}
}

func TestPartitionAdd_AfterInsertion(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	ctx.Partitions = []PartitionDef{
		{Name: "boot", Filesystem: "vfat", Size: 1 << 30},
		{Name: "root", Filesystem: "ext4", Size: 12 << 30},
	}
	execAction(t, config.Step{
		Action: "partition-add",
		PartitionAdd: &config.PartitionAddStep{
			Partitions: []config.Partition{
				{Name: "swap", Filesystem: "swap", Size: "2G", Type: "swap"},
			},
			After: "boot",
		},
	}, ctx)
	if len(ctx.Partitions) != 3 {
		t.Fatalf("Partitions = %d, want 3", len(ctx.Partitions))
	}
	if ctx.Partitions[1].Name != "swap" {
		t.Errorf("partition[1] = %q, want 'swap' (inserted after boot)", ctx.Partitions[1].Name)
	}
}

func TestPartitionAdd_History(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "partition-add",
		PartitionAdd: &config.PartitionAddStep{
			Partitions: []config.Partition{
				{Name: "boot", Filesystem: "vfat", Size: "1G", Type: "efi"},
			},
		},
	}, ctx)
	if len(ctx.PartitionHistory) != 1 {
		t.Errorf("PartitionHistory = %d, want 1", len(ctx.PartitionHistory))
	}
}

func TestPartitionRemove(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test"
	ctx.Partitions = []PartitionDef{
		{Name: "boot"},
		{Name: "root"},
		{Name: "swap"},
	}
	execAction(t, config.Step{
		Action:         "partition-remove",
		PartitionRemove: &config.PartitionRemoveStep{Name: "swap"},
	}, ctx)
	if len(ctx.Partitions) != 2 {
		t.Errorf("Partitions = %d, want 2", len(ctx.Partitions))
	}
	for _, p := range ctx.Partitions {
		if p.Name == "swap" {
			t.Error("swap partition should be removed")
		}
	}
}

func TestPartitionRemove_Missing(t *testing.T) {
	ctx := NewBuildContext()
	ctx.Partitions = []PartitionDef{{Name: "boot"}}
	err := execActionErr(t, config.Step{
		Action:         "partition-remove",
		PartitionRemove: &config.PartitionRemoveStep{Name: "nonexistent"},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing partition")
	}
}

func TestPartitionChange(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "override"
	ctx.Partitions = []PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 12 << 30, Type: "linux"},
	}
	execAction(t, config.Step{
		Action: "partition-change",
		PartitionChange: &config.PartitionChangeStep{
			Name: "root",
			Size: "20G",
		},
	}, ctx)
	if ctx.Partitions[0].Size != 20<<30 {
		t.Errorf("root size = %d, want %d", ctx.Partitions[0].Size, uint64(20<<30))
	}
	// Filesystem unchanged
	if ctx.Partitions[0].Filesystem != "ext4" {
		t.Errorf("filesystem changed unexpectedly")
	}
}

func TestPartitionChange_OnlySpecifiedFields(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test"
	ctx.Partitions = []PartitionDef{
		{Name: "data", Filesystem: "ext4", Size: 256 << 20, MountPoint: "/data", Type: "linux", Grow: true},
	}
	execAction(t, config.Step{
		Action: "partition-change",
		PartitionChange: &config.PartitionChangeStep{
			Name:       "data",
			MountPoint: "/mnt/data",
		},
	}, ctx)
	if ctx.Partitions[0].MountPoint != "/mnt/data" {
		t.Errorf("MountPoint = %q", ctx.Partitions[0].MountPoint)
	}
	// Other fields unchanged
	if ctx.Partitions[0].Filesystem != "ext4" {
		t.Error("Filesystem changed")
	}
	if ctx.Partitions[0].Size != 256<<20 {
		t.Error("Size changed")
	}
}

// --- System Config ---

func TestSystemHostname(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: hostname: telemetryos-edge
	execAction(t, config.Step{
		Action:         "system-hostname",
		SystemHostname: &config.SystemHostnameStep{Hostname: "telemetryos-edge"},
	}, ctx)
	if ctx.Hostname != "telemetryos-edge" {
		t.Errorf("Hostname = %q", ctx.Hostname)
	}
	if len(ctx.HostnameHistory) != 1 || ctx.HostnameHistory[0].Value != "telemetryos-edge" {
		t.Errorf("HostnameHistory = %+v", ctx.HostnameHistory)
	}
}

func TestSystemHostname_MissingError(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:         "system-hostname",
		SystemHostname: &config.SystemHostnameStep{Hostname: ""},
	}, ctx)
	if err == nil {
		t.Error("expected error for empty hostname")
	}
}

func TestSystemTimezone(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:         "system-timezone",
		SystemTimezone: &config.SystemTimezoneStep{Timezone: "UTC"},
	}, ctx)
	if ctx.Timezone != "UTC" {
		t.Errorf("Timezone = %q", ctx.Timezone)
	}
	if len(ctx.TimezoneHistory) != 1 {
		t.Error("TimezoneHistory not recorded")
	}
}

func TestSystemKeymap(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:       "system-keymap",
		SystemKeymap: &config.SystemKeymapStep{Keymap: "us"},
	}, ctx)
	if ctx.Keymap != "us" {
		t.Errorf("Keymap = %q", ctx.Keymap)
	}
}

func TestSystemLocale(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: locale: en_US.UTF-8
	execAction(t, config.Step{
		Action: "system-locale",
		SystemLocale: &config.SystemLocaleStep{
			Locale:  "en_US.UTF-8",
			Locales: []string{"en_US.UTF-8", "de_DE.UTF-8"},
		},
	}, ctx)
	if ctx.Locale != "en_US.UTF-8" {
		t.Errorf("Locale = %q", ctx.Locale)
	}
	if len(ctx.Locales) != 2 {
		t.Errorf("Locales = %v", ctx.Locales)
	}
}

func TestSystemLocale_LocaleOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:       "system-locale",
		SystemLocale: &config.SystemLocaleStep{Locale: "en_US.UTF-8"},
	}, ctx)
	if ctx.Locale != "en_US.UTF-8" {
		t.Errorf("Locale = %q", ctx.Locale)
	}
	if len(ctx.Locales) != 0 {
		t.Errorf("Locales should be empty, got %v", ctx.Locales)
	}
}

// --- Users and Groups ---

func TestSystemUser_New(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: player user with 10 groups
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "player",
			Groups: config.Mergeable[[]string]{Value: []string{"wheel", "video", "render", "seat", "audio", "input", "data", "docker", "lp", "network"}},
			Shell:  "/bin/bash",
		},
	}, ctx)
	if len(ctx.Users) != 1 {
		t.Fatalf("Users = %d", len(ctx.Users))
	}
	if ctx.Users[0].Name != "player" {
		t.Errorf("Name = %q", ctx.Users[0].Name)
	}
	if len(ctx.Users[0].Groups) != 10 {
		t.Errorf("Groups = %v", ctx.Users[0].Groups)
	}
}

func TestSystemUser_MergeAddGroups(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"

	// Base layer: staff with [wheel]
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:     "staff",
			Groups:   config.Mergeable[[]string]{Value: []string{"wheel"}},
			Shell:    "/bin/bash",
			Password: "fiber-buffer-deploy-vault",
		},
	}, ctx)

	ctx.CurrentLayer = "development"
	// Dev layer: !add [player] — matches Edge-OS development layer
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:       "staff",
			Groups:     config.Mergeable[[]string]{Value: []string{"player"}, Mode: config.ModeAdd},
			Shell:      "/usr/bin/fish",
			NoPassword: true,
		},
	}, ctx)

	if len(ctx.Users) != 1 {
		t.Fatalf("expected 1 user after merge, got %d", len(ctx.Users))
	}
	staff := ctx.Users[0]
	if len(staff.Groups) != 2 || staff.Groups[0] != "wheel" || staff.Groups[1] != "player" {
		t.Errorf("Groups after !add = %v, want [wheel player]", staff.Groups)
	}
	if staff.Shell != "/usr/bin/fish" {
		t.Errorf("Shell = %q, want /usr/bin/fish", staff.Shell)
	}
	if !staff.NoPassword {
		t.Error("NoPassword should be true after dev layer")
	}
	if staff.Password != "" {
		t.Errorf("Password should be cleared when NoPassword set, got %q", staff.Password)
	}
}

func TestSystemUser_MergeRemoveGroups(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "player",
			Groups: config.Mergeable[[]string]{Value: []string{"wheel", "video", "docker"}},
		},
	}, ctx)

	ctx.CurrentLayer = "override"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "player",
			Groups: config.Mergeable[[]string]{Value: []string{"docker"}, Mode: config.ModeRemove},
		},
	}, ctx)

	if len(ctx.Users[0].Groups) != 2 {
		t.Errorf("Groups after !remove = %v", ctx.Users[0].Groups)
	}
	for _, g := range ctx.Users[0].Groups {
		if g == "docker" {
			t.Error("docker group should be removed")
		}
	}
}

func TestSystemUser_MergeReplaceGroups(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "player",
			Groups: config.Mergeable[[]string]{Value: []string{"wheel", "video"}},
		},
	}, ctx)

	ctx.CurrentLayer = "override"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "player",
			Groups: config.Mergeable[[]string]{Value: []string{"newgroup"}, Mode: config.ModeReplace},
		},
	}, ctx)

	if len(ctx.Users[0].Groups) != 1 || ctx.Users[0].Groups[0] != "newgroup" {
		t.Errorf("Groups after replace = %v", ctx.Users[0].Groups)
	}
}

func TestSystemUser_PasswordOverridesNoPassword(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:       "test",
			NoPassword: true,
		},
	}, ctx)

	ctx.CurrentLayer = "secure"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:     "test",
			Password: "secret",
		},
	}, ctx)

	if ctx.Users[0].NoPassword {
		t.Error("NoPassword should be false after password set")
	}
	if ctx.Users[0].Password != "secret" {
		t.Errorf("Password = %q", ctx.Users[0].Password)
	}
}

func TestSystemUser_SystemUser(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: data system user
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "data",
			System: true,
			Shell:  "/usr/bin/nologin",
		},
	}, ctx)
	if !ctx.Users[0].System {
		t.Error("System should be true")
	}
	if ctx.Users[0].Shell != "/usr/bin/nologin" {
		t.Errorf("Shell = %q", ctx.Users[0].Shell)
	}
}

func TestSystemGroup_New(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:      "system-group",
		SystemGroup: &config.SystemGroupStep{Name: "docker", System: true},
	}, ctx)
	if len(ctx.Groups) != 1 {
		t.Fatalf("Groups = %d", len(ctx.Groups))
	}
	if ctx.Groups[0].Name != "docker" || !ctx.Groups[0].System {
		t.Errorf("Group = %+v", ctx.Groups[0])
	}
}

func TestSystemGroup_ReplaceOnName(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:      "system-group",
		SystemGroup: &config.SystemGroupStep{Name: "docker", GID: 100},
	}, ctx)
	ctx.CurrentLayer = "override"
	execAction(t, config.Step{
		Action:      "system-group",
		SystemGroup: &config.SystemGroupStep{Name: "docker", GID: 200},
	}, ctx)
	if len(ctx.Groups) != 1 {
		t.Fatalf("Groups = %d, want 1 (replaced)", len(ctx.Groups))
	}
	if ctx.Groups[0].GID != 200 {
		t.Errorf("GID = %d, want 200", ctx.Groups[0].GID)
	}
}

// --- Files ---

func TestFileCreate_Inline(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "file-create",
		FileCreate: &config.FileCreateStep{
			Path:    "/etc/os_release",
			Content: "TelemetryOS Edge 1.0\n",
		},
	}, ctx)
	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates = %d", len(ctx.FileCreates))
	}
	if ctx.FileCreates[0].Mode != "0644" {
		t.Errorf("default mode = %q, want '0644'", ctx.FileCreates[0].Mode)
	}
}

func TestFileCreate_CustomMode(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: sudoers files with mode 0440
	execAction(t, config.Step{
		Action: "file-create",
		FileCreate: &config.FileCreateStep{
			Path:    "/etc/sudoers.d/wheel",
			Content: "%wheel ALL=(ALL:ALL) ALL\n",
			Mode:    "0440",
		},
	}, ctx)
	if ctx.FileCreates[0].Mode != "0440" {
		t.Errorf("mode = %q, want '0440'", ctx.FileCreates[0].Mode)
	}
}

func TestFileCreate_ReplaceOnPath(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:     "file-create",
		FileCreate: &config.FileCreateStep{Path: "/etc/test", Content: "v1"},
	}, ctx)
	ctx.CurrentLayer = "override"
	execAction(t, config.Step{
		Action:     "file-create",
		FileCreate: &config.FileCreateStep{Path: "/etc/test", Content: "v2"},
	}, ctx)
	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates = %d, want 1 (replaced)", len(ctx.FileCreates))
	}
	if ctx.FileCreates[0].Content != "v2" {
		t.Errorf("Content = %q, want 'v2'", ctx.FileCreates[0].Content)
	}
}

func TestFileCreate_MutualExclusion(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action: "file-create",
		FileCreate: &config.FileCreateStep{
			Path:      "/etc/test",
			Content:   "inline",
			LayerPath: "./files/test",
		},
	}, ctx)
	if err == nil {
		t.Error("expected error for layer_path + content")
	}
}

func TestFileCreate_LayerPath(t *testing.T) {
	// Create a temp layer dir with a file
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "test.conf"), []byte("file content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	a, _ := Get("file-create")
	err := a.Execute(config.Step{
		Action: "file-create",
		FileCreate: &config.FileCreateStep{
			Path:      "/etc/test.conf",
			LayerPath: "test.conf",
		},
	}, tmpDir, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates = %d", len(ctx.FileCreates))
	}
	if ctx.FileCreates[0].Content != "file content" {
		t.Errorf("Content = %q", ctx.FileCreates[0].Content)
	}
}

func TestFileCopy(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:   "file-copy",
		FileCopy: &config.FileCopyStep{FromPath: "/src", ToPath: "/dst"},
	}, ctx)
	if len(ctx.FileCopies) != 1 {
		t.Fatalf("InternalCopies = %d", len(ctx.FileCopies))
	}
}

func TestFileCopy_MissingPaths(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:   "file-copy",
		FileCopy: &config.FileCopyStep{FromPath: "", ToPath: "/dst"},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing from_path")
	}
	err = execActionErr(t, config.Step{
		Action:   "file-copy",
		FileCopy: &config.FileCopyStep{FromPath: "/src", ToPath: ""},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing to_path")
	}
}

func TestFileMove(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:   "file-move",
		FileMove: &config.FileMoveStep{FromPath: "/old", ToPath: "/new"},
	}, ctx)
	if len(ctx.FileMoves) != 1 {
		t.Errorf("Moves = %d", len(ctx.FileMoves))
	}
}

func TestFileDelete(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:     "file-delete",
		FileDelete: &config.FileDeleteStep{Path: "/tmp/old", Recursive: true},
	}, ctx)
	if len(ctx.FileDeletes) != 1 {
		t.Fatalf("Removes = %d", len(ctx.FileDeletes))
	}
	if !ctx.FileDeletes[0].Recursive {
		t.Error("Recursive should be true")
	}
}

func TestFileLink(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:   "file-link",
		FileLink: &config.FileLinkStep{FromPath: "/usr/bin/target", ToPath: "/usr/local/bin/link"},
	}, ctx)
	if len(ctx.FileLinks) != 1 {
		t.Fatalf("Links = %d", len(ctx.FileLinks))
	}
	if ctx.FileLinks[0].Type != "symbolic" {
		t.Errorf("default link type = %q, want 'symbolic'", ctx.FileLinks[0].Type)
	}
}

func TestFileLink_Hard(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:   "file-link",
		FileLink: &config.FileLinkStep{FromPath: "/a", ToPath: "/b", Type: "hard"},
	}, ctx)
	if ctx.FileLinks[0].Type != "hard" {
		t.Errorf("link type = %q, want 'hard'", ctx.FileLinks[0].Type)
	}
}

func TestFileMkdir(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action:    "file-mkdir",
		FileMkdir: &config.FileMkdirStep{Path: "/data", Owner: "data", Group: "data", Mode: "2775"},
	}, ctx)
	if len(ctx.FileMkdirs) != 1 {
		t.Fatalf("Mkdirs = %d", len(ctx.FileMkdirs))
	}
	if ctx.FileMkdirs[0].Mode != "2775" {
		t.Errorf("Mode = %q", ctx.FileMkdirs[0].Mode)
	}
}

func TestFilePermissions(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: /data with mode 2775
	execAction(t, config.Step{
		Action:          "file-permissions",
		FilePermissions: &config.FilePermissionsStep{Path: "/data", Mode: "2775"},
	}, ctx)
	if len(ctx.FilePermissions) != 1 {
		t.Fatalf("Permissions = %d", len(ctx.FilePermissions))
	}
	if ctx.FilePermissions[0].Mode != "2775" {
		t.Errorf("Mode = %q", ctx.FilePermissions[0].Mode)
	}
}

func TestFilePermissions_MissingMode(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:          "file-permissions",
		FilePermissions: &config.FilePermissionsStep{Path: "/data", Mode: ""},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing mode")
	}
}

func TestFileOwnership(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: /data owned by data:data
	execAction(t, config.Step{
		Action:        "file-ownership",
		FileOwnership: &config.FileOwnershipStep{Path: "/data", Owner: "data", Group: "data"},
	}, ctx)
	if len(ctx.FileOwnerships) != 1 {
		t.Fatalf("Ownerships = %d", len(ctx.FileOwnerships))
	}
}

func TestFileOwnership_Recursive(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "dev"
	// Edge-OS: /home/staff recursive
	execAction(t, config.Step{
		Action:        "file-ownership",
		FileOwnership: &config.FileOwnershipStep{Path: "/home/staff", Owner: "staff", Group: "staff", Recursive: true},
	}, ctx)
	if !ctx.FileOwnerships[0].Recursive {
		t.Error("Recursive should be true")
	}
}

func TestFileOwnership_MissingBoth(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:        "file-ownership",
		FileOwnership: &config.FileOwnershipStep{Path: "/data"},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing owner and group")
	}
}

// --- File Edit ---

func TestFileEdit_AppendTag(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "file-edit",
		FileEdit: &config.FileEditStep{
			Path:    "/etc/test",
			Content: config.TaggedContent{Tag: "append", Value: "new line"},
		},
	}, ctx)
	if len(ctx.FileEdits) != 1 {
		t.Fatalf("FileEdits = %d", len(ctx.FileEdits))
	}
	if ctx.FileEdits[0].Insert != "append" {
		t.Errorf("Insert = %q", ctx.FileEdits[0].Insert)
	}
}

func TestFileEdit_BeforeTag(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "file-edit",
		FileEdit: &config.FileEditStep{
			Path: "/etc/test",
			Content: config.TaggedContent{
				Tag:     "before",
				Pattern: "^\\[section\\]",
				Value:   "inserted",
				Match:   1,
			},
		},
	}, ctx)
	if ctx.FileEdits[0].Insert != "before" {
		t.Errorf("Insert = %q", ctx.FileEdits[0].Insert)
	}
	if ctx.FileEdits[0].Pattern != "^\\[section\\]" {
		t.Errorf("Pattern = %q", ctx.FileEdits[0].Pattern)
	}
	if ctx.FileEdits[0].Match != 1 {
		t.Errorf("Match = %d", ctx.FileEdits[0].Match)
	}
}

func TestFileEdit_TruncateTag(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "file-edit",
		FileEdit: &config.FileEditStep{
			Path: "/etc/test",
			Content: config.TaggedContent{
				Tag:     "truncate_before",
				Pattern: "^# START",
			},
		},
	}, ctx)
	if ctx.FileEdits[0].Truncate != "truncate_before" {
		t.Errorf("Truncate = %q", ctx.FileEdits[0].Truncate)
	}
}

func TestFileEdit_LegacyFields(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "file-edit",
		FileEdit: &config.FileEditStep{
			Path:    "/etc/test",
			Content: config.TaggedContent{Value: "content"},
			Insert:  "append",
			Pattern: "^foo",
			Match:   2,
		},
	}, ctx)
	if ctx.FileEdits[0].Insert != "append" {
		t.Errorf("Insert = %q", ctx.FileEdits[0].Insert)
	}
	if ctx.FileEdits[0].Pattern != "^foo" {
		t.Errorf("Pattern = %q", ctx.FileEdits[0].Pattern)
	}
}

// --- Systemd Units ---

func TestSystemdService_EnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: enable NetworkManager, sshd, seatd, etc.
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name:   "NetworkManager",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "NetworkManager.service" {
		t.Errorf("Enable = %v", ctx.Services.Enable)
	}
}

func TestSystemdService_Mask(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name: "test",
			Mask: true,
		},
	}, ctx)
	if len(ctx.Services.Mask) != 1 || ctx.Services.Mask[0] != "test.service" {
		t.Errorf("Mask = %v", ctx.Services.Mask)
	}
}

func TestSystemdService_InlineContent(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "player"
	// Edge-OS player service with inline unit sections
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name:   "player",
			User:   "player",
			Enable: true,
			UnitSec: config.UnitSection{
				"Description": "TelemetryOS Player",
				"After":       "sway-session.target",
			},
			Service: config.UnitSection{
				"Type":      "notify",
				"ExecStart": "/home/player/.local/share/player/tos-player",
			},
			Install: config.UnitSection{
				"WantedBy": "sway-session.target",
			},
		},
	}, ctx)

	// Should create a file in user dir
	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates = %d, want 1", len(ctx.FileCreates))
	}
	if !strings.Contains(ctx.FileCreates[0].Path, "/home/player/.config/systemd/user/player.service") {
		t.Errorf("Path = %q", ctx.FileCreates[0].Path)
	}
	// Should also add to user enable
	if len(ctx.Services.UserEnable) != 1 {
		t.Errorf("UserEnable = %d", len(ctx.Services.UserEnable))
	}
	if ctx.Services.UserEnable[0].User != "player" {
		t.Errorf("UserEnable.User = %q", ctx.Services.UserEnable[0].User)
	}
}

func TestSystemdService_UserEnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "player"
	// Edge-OS: enable pipewire for player user
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name:   "pipewire",
			User:   "player",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.UserEnable) != 1 {
		t.Fatalf("UserEnable = %d", len(ctx.Services.UserEnable))
	}
	if ctx.Services.UserEnable[0].Service != "pipewire.service" {
		t.Errorf("Service = %q", ctx.Services.UserEnable[0].Service)
	}
	if ctx.Services.UserEnable[0].User != "player" {
		t.Errorf("User = %q", ctx.Services.UserEnable[0].User)
	}
}

func TestSystemdService_ExtendsDropin(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: getty@tty1 autologin drop-in
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name: "autologin.conf",
			Extends: &config.ExtendsRef{
				Type: "service",
				Name: "getty@tty1",
			},
			Service: config.UnitSection{
				"ExecStart": config.ReplaceValue{Value: "-/sbin/agetty --autologin player"},
			},
		},
	}, ctx)
	if len(ctx.FileCreates) != 1 {
		t.Fatalf("FileCreates = %d", len(ctx.FileCreates))
	}
	expected := "/etc/systemd/system/getty@tty1.service.d/autologin.conf"
	if ctx.FileCreates[0].Path != expected {
		t.Errorf("Path = %q, want %q", ctx.FileCreates[0].Path, expected)
	}
}

func TestSystemdService_AutoExtension(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name:   "sshd",
			Enable: true,
		},
	}, ctx)
	// "sshd" → "sshd.service"
	if ctx.Services.Enable[0] != "sshd.service" {
		t.Errorf("Enable = %q, want sshd.service", ctx.Services.Enable[0])
	}
}

func TestSystemdMount_EnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-mount",
		SystemdMount: &config.SystemdMountStep{
			Name:   "data",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "data.mount" {
		t.Errorf("Enable = %v", ctx.Services.Enable)
	}
}

func TestSystemdTimer_EnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: fstrim timer
	execAction(t, config.Step{
		Action: "systemd-timer",
		SystemdTimer: &config.SystemdTimerStep{
			Name:   "fstrim",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "fstrim.timer" {
		t.Errorf("Enable = %v", ctx.Services.Enable)
	}
}

func TestSystemdSocket_EnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-socket",
		SystemdSocket: &config.SystemdSocketStep{
			Name:   "dbus",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "dbus.socket" {
		t.Errorf("Enable = %v", ctx.Services.Enable)
	}
}

func TestSystemdSlice_EnableOnly(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-slice",
		SystemdSlice: &config.SystemdSliceStep{
			Name:   "user",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "user.slice" {
		t.Errorf("Enable = %v", ctx.Services.Enable)
	}
}

func TestSystemdTarget_SetDefault(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: target: multi-user.target
	execAction(t, config.Step{
		Action:        "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{Target: "multi-user.target"},
	}, ctx)
	if ctx.DefaultTarget != "multi-user.target" {
		t.Errorf("DefaultTarget = %q", ctx.DefaultTarget)
	}
	if len(ctx.DefaultTargetHistory) != 1 {
		t.Errorf("DefaultTargetHistory = %d", len(ctx.DefaultTargetHistory))
	}
}

func TestSystemdTarget_EnableUnit(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{
			Name:   "graphical",
			Enable: true,
		},
	}, ctx)
	if len(ctx.Services.Enable) != 1 || ctx.Services.Enable[0] != "graphical.target" {
		t.Errorf("Enable = %v", ctx.Services.Enable)
	}
}

func TestSystemdBootInstall(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	// Edge-OS: systemd-boot with 3 entries
	execAction(t, config.Step{
		Action: "systemd-boot-install",
		SystemdBootInstall: &config.SystemdBootInstallStep{
			Loader: &config.BootLoader{
				Default: "arch.conf",
				Timeout: 0,
				Editor:  false,
			},
			Entries: []config.BootEntry{
				{Name: "arch.conf", Title: "TelemetryOS Edge", Linux: "/vmlinuz-linux", Initrd: "/initramfs-linux.img", Options: "rw quiet splash"},
				{Name: "recovery.conf", Title: "TelemetryOS Recovery", Linux: "/vmlinuz-linux", Initrd: "/initramfs-linux.img", Options: "rw quiet"},
			},
		},
	}, ctx)
	if ctx.Boot == nil {
		t.Fatal("Boot is nil")
	}
	if ctx.Boot.Loader.Default != "arch.conf" {
		t.Errorf("Loader.Default = %q", ctx.Boot.Loader.Default)
	}
	if len(ctx.Boot.Entries) != 2 {
		t.Errorf("Entries = %d", len(ctx.Boot.Entries))
	}
}

func TestSystemdBootInstall_EntriesAccumulate(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "systemd-boot-install",
		SystemdBootInstall: &config.SystemdBootInstallStep{
			Entries: []config.BootEntry{{Name: "arch.conf", Title: "Main"}},
		},
	}, ctx)
	execAction(t, config.Step{
		Action: "systemd-boot-install",
		SystemdBootInstall: &config.SystemdBootInstallStep{
			Entries: []config.BootEntry{{Name: "recovery.conf", Title: "Recovery"}},
		},
	}, ctx)
	if len(ctx.Boot.Entries) != 2 {
		t.Errorf("Entries = %d, want 2 (accumulated)", len(ctx.Boot.Entries))
	}
}

// --- Scripts ---

func TestRun_Inline(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "dev"
	// Edge-OS development layer: inline scripts
	execAction(t, config.Step{
		Action: "run",
		Run: &config.RunStep{
			Script: "#!/bin/bash\necho hello",
			User:   "staff",
		},
	}, ctx)
	if len(ctx.Scripts) != 1 {
		t.Fatalf("Scripts = %d", len(ctx.Scripts))
	}
	if ctx.Scripts[0].Content != "#!/bin/bash\necho hello" {
		t.Errorf("Content = %q", ctx.Scripts[0].Content)
	}
	if ctx.Scripts[0].User != "staff" {
		t.Errorf("User = %q", ctx.Scripts[0].User)
	}
}

func TestRun_ScriptPath(t *testing.T) {
	// Create temp script file
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "setup.sh"), []byte("#!/bin/bash\nsetup"), 0755); err != nil {
		t.Fatal(err)
	}
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test"
	a, _ := Get("run")
	err := a.Execute(config.Step{
		Action: "run",
		Run:    &config.RunStep{ScriptPath: "setup.sh"},
	}, tmpDir, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Scripts) != 1 {
		t.Fatal("expected 1 script")
	}
	if ctx.Scripts[0].Script != "setup.sh" {
		t.Errorf("Script = %q, want 'setup.sh'", ctx.Scripts[0].Script)
	}
}

func TestRun_MutuallyExclusive(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action: "run",
		Run:    &config.RunStep{Script: "echo hi", ScriptPath: "file.sh"},
	}, ctx)
	if err == nil {
		t.Error("expected error for script + script_path")
	}
}

func TestRun_MissingBoth(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action: "run",
		Run:    &config.RunStep{},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing script and script_path")
	}
}

func TestRun_EnvPassthrough(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "test"
	execAction(t, config.Step{
		Action: "run",
		Run: &config.RunStep{
			Script: "echo $FOO",
			Env:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
	}, ctx)
	if ctx.Scripts[0].Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %q", ctx.Scripts[0].Env["FOO"])
	}
	if ctx.Scripts[0].Env["BAZ"] != "qux" {
		t.Errorf("Env[BAZ] = %q", ctx.Scripts[0].Env["BAZ"])
	}
}

// --- Installer ---

func TestInstallServer_Defaults(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "installer"
	execAction(t, config.Step{
		Action:        "install-server",
		InstallServer: &config.InstallServerStep{},
	}, ctx)
	if ctx.InstallerServer == nil {
		t.Fatal("InstallerServer is nil")
	}
	if ctx.InstallerServer.Port != 8100 {
		t.Errorf("Port = %d, want 8100", ctx.InstallerServer.Port)
	}
	if ctx.InstallerServer.Path != "/usr/lib/starforge/payloads" {
		t.Errorf("Path = %q", ctx.InstallerServer.Path)
	}
	// Should add runtime deps to packages
	if len(ctx.Packages) < 3 {
		t.Errorf("Packages = %v (should contain installer deps)", ctx.Packages)
	}
	found := map[string]bool{}
	for _, p := range ctx.Packages {
		found[p] = true
	}
	for _, dep := range []string{"dosfstools", "e2fsprogs", "zstd"} {
		if !found[dep] {
			t.Errorf("missing installer dep %q in Packages", dep)
		}
	}
}

func TestInstallServer_CustomPortPath(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "installer"
	execAction(t, config.Step{
		Action:        "install-server",
		InstallServer: &config.InstallServerStep{Port: 9090, Path: "/images"},
	}, ctx)
	if ctx.InstallerServer.Port != 9090 {
		t.Errorf("Port = %d", ctx.InstallerServer.Port)
	}
	if ctx.InstallerServer.Path != "/images" {
		t.Errorf("Path = %q", ctx.InstallerServer.Path)
	}
}

func TestInstallClient(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "installer"
	execAction(t, config.Step{
		Action:        "install-client",
		InstallClient: &config.InstallClientStep{AutoLogin: "installer"},
	}, ctx)
	if ctx.InstallerClient == nil {
		t.Fatal("InstallerClient is nil")
	}
	if ctx.InstallerClient.AutoLogin != "installer" {
		t.Errorf("AutoLogin = %q", ctx.InstallerClient.AutoLogin)
	}
}

func TestInstallPayload(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "installer"
	// Edge-OS: install-payload target: device, path: /images/device
	execAction(t, config.Step{
		Action:         "install-payload",
		InstallPayload: &config.InstallPayloadStep{Target: "device", Path: "/images/device"},
	}, ctx)
	if len(ctx.InstallerPayloads) != 1 {
		t.Fatalf("InstallerPayloads = %d", len(ctx.InstallerPayloads))
	}
	if ctx.InstallerPayloads[0].Target != "device" {
		t.Errorf("Target = %q", ctx.InstallerPayloads[0].Target)
	}
	if ctx.InstallerPayloads[0].Path != "/images/device" {
		t.Errorf("Path = %q", ctx.InstallerPayloads[0].Path)
	}
}

func TestInstallPayload_MissingTarget(t *testing.T) {
	ctx := NewBuildContext()
	err := execActionErr(t, config.Step{
		Action:         "install-payload",
		InstallPayload: &config.InstallPayloadStep{Target: ""},
	}, ctx)
	if err == nil {
		t.Error("expected error for missing target")
	}
}

func TestInstallPayload_Accumulates(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "installer"
	execAction(t, config.Step{
		Action:         "install-payload",
		InstallPayload: &config.InstallPayloadStep{Target: "device", Path: "/images/device"},
	}, ctx)
	execAction(t, config.Step{
		Action:         "install-payload",
		InstallPayload: &config.InstallPayloadStep{Target: "device-dev", Path: "/images/device-dev"},
	}, ctx)
	if len(ctx.InstallerPayloads) != 2 {
		t.Errorf("InstallerPayloads = %d, want 2", len(ctx.InstallerPayloads))
	}
}

// === Edge-OS Integration Tests ===
// These simulate processing the actual Edge-OS layer steps and verify
// the resulting BuildContext matches expected values.

func TestEdgeOS_BaseLayer(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "base"

	// --- partition-add: 6 partitions matching Edge-OS base ---
	execAction(t, config.Step{
		Action: "partition-add",
		PartitionAdd: &config.PartitionAddStep{
			Partitions: []config.Partition{
				{Name: "boot", Filesystem: "vfat", Size: "1G", MountPoint: "/boot", Type: "efi"},
				{Name: "root", Filesystem: "ext4", Size: "12G", MountPoint: "/"},
				{Name: "fallback-recovery", Filesystem: "ext4", Size: "6G", MountPoint: "/fallback-recovery"},
				{Name: "recovery", Filesystem: "ext4", Size: "6G", MountPoint: "/recovery"},
				{Name: "logs", Filesystem: "ext4", Size: "512M", MountPoint: "/var/log"},
				{Name: "data", Filesystem: "ext4", Size: "256M+", MountPoint: "/data"},
			},
		},
	}, ctx)

	// --- pacman-add: 9 base packages ---
	execAction(t, config.Step{
		Action: "pacman-add",
		PacmanAdd: &config.PacmanAddStep{
			Packages: []string{"base", "linux", "linux-firmware", "sudo", "networkmanager", "openssh", "plymouth", "intel-ucode", "util-linux"},
		},
	}, ctx)

	// --- system config ---
	execAction(t, config.Step{
		Action:         "system-hostname",
		SystemHostname: &config.SystemHostnameStep{Hostname: "telemetryos-edge"},
	}, ctx)
	execAction(t, config.Step{
		Action:       "system-locale",
		SystemLocale: &config.SystemLocaleStep{Locale: "en_US.UTF-8"},
	}, ctx)
	execAction(t, config.Step{
		Action:         "system-timezone",
		SystemTimezone: &config.SystemTimezoneStep{Timezone: "UTC"},
	}, ctx)
	execAction(t, config.Step{
		Action:       "system-keymap",
		SystemKeymap: &config.SystemKeymapStep{Keymap: "us"},
	}, ctx)
	execAction(t, config.Step{
		Action:        "systemd-target",
		SystemdTarget: &config.SystemdTargetStep{Target: "multi-user.target"},
	}, ctx)

	// --- boot config ---
	execAction(t, config.Step{
		Action: "systemd-boot-install",
		SystemdBootInstall: &config.SystemdBootInstallStep{
			Loader: &config.BootLoader{Default: "arch.conf", Timeout: 0, Editor: false},
			Entries: []config.BootEntry{
				{Name: "arch.conf", Title: "TelemetryOS Edge", Linux: "/vmlinuz-linux", Initrd: "/initramfs-linux.img", Options: "rw quiet splash rootflags=noatime,commit=600 audit=0 noresume"},
				{Name: "recovery.conf", Title: "TelemetryOS Recovery", Linux: "/vmlinuz-linux", Initrd: "/initramfs-linux.img", Options: "rw quiet rootflags=noatime,commit=600 noresume"},
				{Name: "fallback-recovery.conf", Title: "TelemetryOS Fallback Recovery", Linux: "/vmlinuz-linux", Initrd: "/initramfs-linux.img", Options: "rw quiet rootflags=noatime,commit=600 noresume"},
			},
		},
	}, ctx)

	// --- users ---
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name: "data", System: true, Shell: "/usr/bin/nologin",
		},
	}, ctx)
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:   "player",
			Groups: config.Mergeable[[]string]{Value: []string{"wheel", "video", "render", "seat", "audio", "input", "data", "docker", "lp", "network"}},
			Shell:  "/bin/bash",
		},
	}, ctx)
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:     "staff",
			Groups:   config.Mergeable[[]string]{Value: []string{"wheel"}},
			Shell:    "/bin/bash",
			Password: "fiber-buffer-deploy-vault",
		},
	}, ctx)

	// --- enable services ---
	for _, svc := range []string{"NetworkManager", "sshd", "seatd", "bluetooth", "systemd-timesyncd", "systemd-resolved", "plymouth-start"} {
		execAction(t, config.Step{
			Action:         "systemd-service",
			SystemdService: &config.SystemdServiceStep{Name: svc, Enable: true},
		}, ctx)
	}
	execAction(t, config.Step{
		Action:       "systemd-timer",
		SystemdTimer: &config.SystemdTimerStep{Name: "fstrim", Enable: true},
	}, ctx)

	// --- inline file-creates (just a few representative ones) ---
	execAction(t, config.Step{
		Action:     "file-create",
		FileCreate: &config.FileCreateStep{Path: "/etc/systemd/journald.conf.d/size-limit.conf", Content: "[Journal]\nSystemMaxUse=100M\n"},
	}, ctx)
	execAction(t, config.Step{
		Action:     "file-create",
		FileCreate: &config.FileCreateStep{Path: "/etc/sudoers.d/wheel", Content: "%wheel ALL=(ALL:ALL) ALL\n", Mode: "0440"},
	}, ctx)

	// --- getty autologin drop-in ---
	execAction(t, config.Step{
		Action: "systemd-service",
		SystemdService: &config.SystemdServiceStep{
			Name:    "autologin.conf",
			Extends: &config.ExtendsRef{Type: "service", Name: "getty@tty1"},
			Service: config.UnitSection{
				"ExecStart": config.ReplaceValue{Value: "-/sbin/agetty --autologin player"},
			},
		},
	}, ctx)

	// --- permissions ---
	execAction(t, config.Step{
		Action:        "file-ownership",
		FileOwnership: &config.FileOwnershipStep{Path: "/data", Owner: "data", Group: "data"},
	}, ctx)
	execAction(t, config.Step{
		Action:          "file-permissions",
		FilePermissions: &config.FilePermissionsStep{Path: "/data", Mode: "2775"},
	}, ctx)

	// === Verify final state ===

	// Partitions: 6 with correct attributes
	if len(ctx.Partitions) != 6 {
		t.Fatalf("Partitions = %d, want 6", len(ctx.Partitions))
	}
	bootPart := ctx.Partitions[0]
	if bootPart.Name != "boot" || bootPart.Type != "efi" || bootPart.Size != 1<<30 {
		t.Errorf("boot = %+v", bootPart)
	}
	dataPart := ctx.Partitions[5]
	if dataPart.Name != "data" || !dataPart.Grow || dataPart.Size != 256<<20 {
		t.Errorf("data = %+v", dataPart)
	}

	// Packages: 9 base packages
	if len(ctx.Packages) != 9 {
		t.Errorf("Packages = %d, want 9", len(ctx.Packages))
	}

	// System config
	if ctx.Hostname != "telemetryos-edge" {
		t.Errorf("Hostname = %q", ctx.Hostname)
	}
	if ctx.Locale != "en_US.UTF-8" {
		t.Errorf("Locale = %q", ctx.Locale)
	}
	if ctx.Timezone != "UTC" {
		t.Errorf("Timezone = %q", ctx.Timezone)
	}
	if ctx.Keymap != "us" {
		t.Errorf("Keymap = %q", ctx.Keymap)
	}
	if ctx.DefaultTarget != "multi-user.target" {
		t.Errorf("DefaultTarget = %q", ctx.DefaultTarget)
	}

	// Boot
	if ctx.Boot == nil || ctx.Boot.Loader.Default != "arch.conf" {
		t.Error("Boot loader config incorrect")
	}
	if len(ctx.Boot.Entries) != 3 {
		t.Errorf("Boot entries = %d, want 3", len(ctx.Boot.Entries))
	}

	// Users: data, player, staff
	if len(ctx.Users) != 3 {
		t.Fatalf("Users = %d, want 3", len(ctx.Users))
	}
	dataUser := ctx.Users[0]
	if dataUser.Name != "data" || !dataUser.System {
		t.Errorf("data user = %+v", dataUser)
	}
	player := ctx.Users[1]
	if player.Name != "player" || len(player.Groups) != 10 {
		t.Errorf("player user = %+v", player)
	}
	staff := ctx.Users[2]
	if staff.Name != "staff" || staff.Password != "fiber-buffer-deploy-vault" {
		t.Errorf("staff user = %+v", staff)
	}

	// Services enabled: 7 services + 1 timer = 8
	if len(ctx.Services.Enable) != 8 {
		t.Errorf("Services.Enable = %d, want 8: %v", len(ctx.Services.Enable), ctx.Services.Enable)
	}

	// File creates: journald conf + sudoers + autologin drop-in = 3
	if len(ctx.FileCreates) != 3 {
		t.Errorf("FileCreates = %d, want 3", len(ctx.FileCreates))
	}

	// Autologin drop-in path
	foundDropin := false
	for _, fc := range ctx.FileCreates {
		if fc.Path == "/etc/systemd/system/getty@tty1.service.d/autologin.conf" {
			foundDropin = true
		}
	}
	if !foundDropin {
		t.Error("missing getty@tty1 autologin drop-in")
	}

	// Ownership + permissions
	if len(ctx.FileOwnerships) != 1 || ctx.FileOwnerships[0].Owner != "data" {
		t.Errorf("Ownerships = %+v", ctx.FileOwnerships)
	}
	if len(ctx.FilePermissions) != 1 || ctx.FilePermissions[0].Mode != "2775" {
		t.Errorf("Permissions = %+v", ctx.FilePermissions)
	}
}

func TestEdgeOS_DevelopmentLayerOverrides(t *testing.T) {
	ctx := NewBuildContext()

	// --- Simulate base layer state for staff user ---
	ctx.CurrentLayer = "base"
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:     "staff",
			Groups:   config.Mergeable[[]string]{Value: []string{"wheel"}},
			Shell:    "/bin/bash",
			Password: "fiber-buffer-deploy-vault",
		},
	}, ctx)
	ctx.Packages = []string{"base", "linux", "linux-firmware", "sudo", "networkmanager", "openssh", "plymouth", "intel-ucode", "util-linux"}

	// --- Apply development layer ---
	ctx.CurrentLayer = "development"

	// pacman-add: 15 dev packages
	execAction(t, config.Step{
		Action: "pacman-add",
		PacmanAdd: &config.PacmanAddStep{
			Packages: []string{"base-devel", "git", "go", "nodejs", "npm", "fish", "nano", "vim", "neovim", "htop", "btop", "curl", "wget", "unzip"},
		},
	}, ctx)

	// staff user override: shell→fish, no_password, !add [player]
	execAction(t, config.Step{
		Action: "system-user",
		SystemUser: &config.SystemUserStep{
			Name:       "staff",
			Shell:      "/usr/bin/fish",
			NoPassword: true,
			Groups:     config.Mergeable[[]string]{Value: []string{"player"}, Mode: config.ModeAdd},
		},
	}, ctx)

	// run scripts
	execAction(t, config.Step{
		Action: "run",
		Run: &config.RunStep{
			User:   "staff",
			Script: "#!/bin/bash\ncurl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash",
		},
	}, ctx)
	execAction(t, config.Step{
		Action: "run",
		Run: &config.RunStep{
			User:   "staff",
			Script: "#!/bin/bash\ngit clone https://github.com/LazyVim/starter ~/.config/nvim\nrm -rf ~/.config/nvim/.git",
		},
	}, ctx)

	// staff home ownership
	execAction(t, config.Step{
		Action:        "file-ownership",
		FileOwnership: &config.FileOwnershipStep{Path: "/home/staff", Owner: "staff", Group: "staff", Recursive: true},
	}, ctx)

	// === Verify development layer effects ===

	// Packages: 9 base + 14 dev = 23
	if len(ctx.Packages) != 23 {
		t.Errorf("Packages = %d, want 23", len(ctx.Packages))
	}

	// Staff user merged properly
	staff := ctx.Users[0]
	if staff.Shell != "/usr/bin/fish" {
		t.Errorf("staff.Shell = %q, want /usr/bin/fish", staff.Shell)
	}
	if !staff.NoPassword {
		t.Error("staff.NoPassword should be true")
	}
	if staff.Password != "" {
		t.Errorf("staff.Password should be empty, got %q", staff.Password)
	}
	// Groups: [wheel] + !add [player] = [wheel, player]
	if len(staff.Groups) != 2 {
		t.Fatalf("staff.Groups = %v, want [wheel player]", staff.Groups)
	}
	if staff.Groups[0] != "wheel" || staff.Groups[1] != "player" {
		t.Errorf("staff.Groups = %v", staff.Groups)
	}

	// Scripts: 2 run actions with user=staff
	if len(ctx.Scripts) != 2 {
		t.Errorf("Scripts = %d, want 2", len(ctx.Scripts))
	}
	for _, s := range ctx.Scripts {
		if s.User != "staff" {
			t.Errorf("Script user = %q, want 'staff'", s.User)
		}
	}

	// Ownership: /home/staff recursive
	if len(ctx.FileOwnerships) != 1 {
		t.Fatalf("Ownerships = %d", len(ctx.FileOwnerships))
	}
	if !ctx.FileOwnerships[0].Recursive {
		t.Error("staff home ownership should be recursive")
	}
}
