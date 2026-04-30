package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

// --- LoadManifest / Save tests ---

func TestLoadManifest_NonExistent(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Phases == nil {
		t.Fatal("Phases should be initialized, not nil")
	}
	if len(m.Phases) != 0 {
		t.Errorf("Phases should be empty, got %d entries", len(m.Phases))
	}
}

func TestManifest_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	m := &Manifest{
		Version: CacheVersion,
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "abc123", Completed: true},
			"1-packages":   {Hash: "def456", Completed: true},
		},
	}

	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	if loaded.Version != CacheVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CacheVersion)
	}
	if len(loaded.Phases) != 2 {
		t.Fatalf("Phases length = %d, want 2", len(loaded.Phases))
	}
	if loaded.Phases["0-preinstall"].Hash != "abc123" {
		t.Errorf("preinstall hash = %q", loaded.Phases["0-preinstall"].Hash)
	}
	if !loaded.Phases["1-packages"].Completed {
		t.Error("packages should be completed")
	}
}

func TestLoadManifest_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("not json"), 0o644)
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadManifest_NilPhases(t *testing.T) {
	dir := t.TempDir()
	// JSON with no phases field
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":1}`), 0o644)
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Phases == nil {
		t.Error("Phases should be initialized even when missing from JSON")
	}
}

// --- IsPhaseCached tests ---

func TestIsPhaseCached_Matching(t *testing.T) {
	dir := t.TempDir()
	// Create the upper dir the function checks for
	os.MkdirAll(filepath.Join(dir, "0-preinstall", "upper"), 0o755)

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "abc123", Completed: true},
		},
	}

	if !IsPhaseCached(dir, 0, "abc123", manifest) {
		t.Error("expected phase to be cached")
	}
}

func TestIsPhaseCached_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "0-preinstall", "upper"), 0o755)

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "abc123", Completed: true},
		},
	}

	if IsPhaseCached(dir, 0, "different", manifest) {
		t.Error("should not be cached with different hash")
	}
}

func TestIsPhaseCached_NotCompleted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "0-preinstall", "upper"), 0o755)

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "abc123", Completed: false},
		},
	}

	if IsPhaseCached(dir, 0, "abc123", manifest) {
		t.Error("should not be cached if not completed")
	}
}

func TestIsPhaseCached_MissingUpperDir(t *testing.T) {
	dir := t.TempDir()
	// Don't create the upper dir

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "abc123", Completed: true},
		},
	}

	if IsPhaseCached(dir, 0, "abc123", manifest) {
		t.Error("should not be cached without upper dir")
	}
}

func TestIsPhaseCached_NotInManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{Phases: make(map[string]PhaseEntry)}

	if IsPhaseCached(dir, 0, "abc123", manifest) {
		t.Error("should not be cached if not in manifest")
	}
}

// --- InvalidateFrom tests ---

func TestInvalidateFrom(t *testing.T) {
	dir := t.TempDir()

	// Create phase directories
	for _, name := range PhaseNames {
		os.MkdirAll(filepath.Join(dir, name, "upper"), 0o755)
	}

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "a", Completed: true},
			"1-packages":   {Hash: "b", Completed: true},
			"2-sysconfig":  {Hash: "c", Completed: true},
			"3-users":      {Hash: "d", Completed: true},
		},
	}

	// Invalidate from phase 2 onward
	if err := InvalidateFrom(dir, 2, manifest); err != nil {
		t.Fatalf("InvalidateFrom error: %v", err)
	}

	// Phases 0 and 1 should still be in manifest
	if _, ok := manifest.Phases["0-preinstall"]; !ok {
		t.Error("phase 0 should still be in manifest")
	}
	if _, ok := manifest.Phases["1-packages"]; !ok {
		t.Error("phase 1 should still be in manifest")
	}

	// Phases 2+ should be removed from manifest
	if _, ok := manifest.Phases["2-sysconfig"]; ok {
		t.Error("phase 2 should be removed from manifest")
	}
	if _, ok := manifest.Phases["3-users"]; ok {
		t.Error("phase 3 should be removed from manifest")
	}

	// Phase directories 2+ should be deleted
	if _, err := os.Stat(filepath.Join(dir, "2-sysconfig")); !os.IsNotExist(err) {
		t.Error("phase 2 directory should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "3-users")); !os.IsNotExist(err) {
		t.Error("phase 3 directory should be deleted")
	}

	// Phase 0 and 1 directories should still exist
	if _, err := os.Stat(filepath.Join(dir, "0-preinstall")); err != nil {
		t.Error("phase 0 directory should still exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "1-packages")); err != nil {
		t.Error("phase 1 directory should still exist")
	}
}

func TestInvalidateFrom_Phase0(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "a", Completed: true},
		},
	}

	if err := InvalidateFrom(dir, 0, manifest); err != nil {
		t.Fatalf("InvalidateFrom error: %v", err)
	}
	if len(manifest.Phases) != 0 {
		t.Errorf("all phases should be removed, got %d", len(manifest.Phases))
	}
}

// --- HashPhase tests ---

func TestHashPhase_Preinstall(t *testing.T) {
	ctx1 := actions.NewBuildContext()
	ctx1.Keymap = "us"

	ctx2 := actions.NewBuildContext()
	ctx2.Keymap = "us"

	ctx3 := actions.NewBuildContext()
	ctx3.Keymap = "uk"

	h1, err := HashPhase(0, ctx1)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}
	h2, err := HashPhase(0, ctx2)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}
	h3, err := HashPhase(0, ctx3)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}

	if h1 != h2 {
		t.Error("same keymap should produce same hash")
	}
	if h1 == h3 {
		t.Error("different keymap should produce different hash")
	}
}

func TestHashPhase_Packages_OrderIndependent(t *testing.T) {
	ctx1 := actions.NewBuildContext()
	ctx1.Packages = []actions.Package{{Name: "base"}, {Name: "linux"}, {Name: "vim"}}

	ctx2 := actions.NewBuildContext()
	ctx2.Packages = []actions.Package{{Name: "vim"}, {Name: "base"}, {Name: "linux"}}

	h1, _ := HashPhase(1, ctx1)
	h2, _ := HashPhase(1, ctx2)

	if h1 != h2 {
		t.Error("package order should not affect hash (sorted internally)")
	}
}

func TestHashPhase_Sysconfig(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Hostname = "edge-device"
	ctx.Locale = "en_US.UTF-8"
	ctx.Timezone = "America/Toronto"
	ctx.Keymap = "us"

	h, err := HashPhase(2, ctx)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}
	if h == "" {
		t.Error("hash should not be empty")
	}
}

func TestHashPhase_Users(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Users = []actions.UserDef{
		{Name: "player", Groups: []string{"video", "render"}, Shell: "/bin/bash"},
	}
	ctx.Groups = []actions.GroupDef{
		{Name: "player", System: false},
	}

	h, err := HashPhase(3, ctx)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}
	if h == "" {
		t.Error("hash should not be empty")
	}
}

func TestHashPhase_InvalidIndex(t *testing.T) {
	ctx := actions.NewBuildContext()
	_, err := HashPhase(99, ctx)
	if err == nil {
		t.Error("expected error for invalid phase index")
	}
}

func TestHashPhase_Services(t *testing.T) {
	ctx1 := actions.NewBuildContext()
	ctx1.Services.Enable = []string{"NetworkManager.service", "sshd.service"}
	ctx1.Services.Mask = []string{"systemd-networkd.service"}

	ctx2 := actions.NewBuildContext()
	ctx2.Services.Enable = []string{"sshd.service", "NetworkManager.service"} // reversed
	ctx2.Services.Mask = []string{"systemd-networkd.service"}

	h1, _ := HashPhase(6, ctx1)
	h2, _ := HashPhase(6, ctx2)
	if h1 != h2 {
		t.Error("service order should not affect hash")
	}
}

func TestHashPhase_Boot(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Boot = &actions.BootConfig{
		Loader: &config.BootLoader{
			Default: "arch.conf",
			Timeout: 3,
			Editor:  false,
		},
		Entries: []config.BootEntry{
			{
				Name:    "arch.conf",
				Title:   "Arch Linux",
				Kernel:  "linux",
				Options: "root=LABEL=root rw",
			},
		},
	}

	h, err := HashPhase(7, ctx)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}
	if h == "" {
		t.Error("hash should not be empty")
	}
}

func TestHashPhase_Scripts_Inline(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Scripts = []actions.ScriptOp{
		{Content: "echo hello", User: "player"},
	}

	h, err := HashPhase(8, ctx)
	if err != nil {
		t.Fatalf("HashPhase error: %v", err)
	}
	if h == "" {
		t.Error("hash should not be empty")
	}
}

// --- PhaseNames tests ---

func TestPhaseNames(t *testing.T) {
	if len(PhaseNames) != 9 {
		t.Errorf("PhaseNames length = %d, want 9", len(PhaseNames))
	}
	expected := []string{
		"0-preinstall", "1-packages", "2-sysconfig", "3-users",
		"4-files", "5-permissions", "6-services", "7-boot", "8-scripts",
	}
	for i, name := range expected {
		if PhaseNames[i] != name {
			t.Errorf("PhaseNames[%d] = %q, want %q", i, PhaseNames[i], name)
		}
	}
}

// --- hashPath tests ---

func TestHashPath_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	h1, err := hashPath(path)
	if err != nil {
		t.Fatalf("hashPath error: %v", err)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}

	// Same content = same hash
	h2, _ := hashPath(path)
	if h1 != h2 {
		t.Error("same file should produce same hash")
	}

	// Different content = different hash
	os.WriteFile(path, []byte("different"), 0o644)
	h3, _ := hashPath(path)
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
}

func TestHashPath_Directory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0o644)

	h1, err := hashPath(dir)
	if err != nil {
		t.Fatalf("hashPath error: %v", err)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}

	// Modify a file in directory
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("changed"), 0o644)
	h2, _ := hashPath(dir)
	if h1 == h2 {
		t.Error("modified directory should produce different hash")
	}
}

func TestHashPath_NonExistent(t *testing.T) {
	_, err := hashPath("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

// --- PackagingEntry tests ---

func TestManifest_PackagingEntry(t *testing.T) {
	dir := t.TempDir()

	m := &Manifest{
		Version: CacheVersion,
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "aaa", Completed: true},
		},
		Packaging: &PackagingEntry{Hash: "pkg-hash-123", Completed: true},
	}

	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	if loaded.Packaging == nil {
		t.Fatal("Packaging should not be nil after load")
	}
	if loaded.Packaging.Hash != "pkg-hash-123" {
		t.Errorf("Packaging.Hash = %q, want %q", loaded.Packaging.Hash, "pkg-hash-123")
	}
	if !loaded.Packaging.Completed {
		t.Error("Packaging.Completed should be true")
	}
}

func TestManifest_PackagingEntry_Nil(t *testing.T) {
	dir := t.TempDir()

	m := &Manifest{
		Version: CacheVersion,
		Phases:  map[string]PhaseEntry{},
	}

	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	if loaded.Packaging != nil {
		t.Error("Packaging should be nil when omitted")
	}
}

func TestInvalidateFrom_ClearsPackaging(t *testing.T) {
	dir := t.TempDir()

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "a", Completed: true},
			"1-packages":   {Hash: "b", Completed: true},
		},
		Packaging: &PackagingEntry{Hash: "pkg-hash", Completed: true},
	}

	if err := InvalidateFrom(dir, 1, manifest); err != nil {
		t.Fatalf("InvalidateFrom error: %v", err)
	}

	if manifest.Packaging != nil {
		t.Error("InvalidateFrom should clear Packaging")
	}
}

func TestHashPackaging_Deterministic(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Partitions = []actions.PartitionDef{
		{Name: "efi", Filesystem: "vfat", Size: 512 << 20, MountPoint: "/boot", Type: "ef00"},
		{Name: "root", Filesystem: "ext4", Size: 4 << 30, MountPoint: "/", Type: "8300", Grow: true},
	}

	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "aaa", Completed: true},
			"1-packages":   {Hash: "bbb", Completed: true},
		},
	}

	h1 := HashPackaging(manifest, ctx, nil)
	h2 := HashPackaging(manifest, ctx, nil)
	if h1 != h2 {
		t.Error("same inputs should produce same hash")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestHashPackaging_ChangesOnPhaseHash(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Partitions = []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 4 << 30, MountPoint: "/", Type: "8300"},
	}

	m1 := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "aaa", Completed: true},
		},
	}
	m2 := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "zzz", Completed: true},
		},
	}

	h1 := HashPackaging(m1, ctx, nil)
	h2 := HashPackaging(m2, ctx, nil)
	if h1 == h2 {
		t.Error("different phase hash should produce different packaging hash")
	}
}

func TestHashPackaging_ChangesOnPartitionDef(t *testing.T) {
	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "aaa", Completed: true},
		},
	}

	ctx1 := actions.NewBuildContext()
	ctx1.Partitions = []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 4 << 30, MountPoint: "/"},
	}

	ctx2 := actions.NewBuildContext()
	ctx2.Partitions = []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 8 << 30, MountPoint: "/"},
	}

	h1 := HashPackaging(manifest, ctx1, nil)
	h2 := HashPackaging(manifest, ctx2, nil)
	if h1 == h2 {
		t.Error("different partition size should produce different packaging hash")
	}
}

func TestHashPackaging_ChangesOnInstaller(t *testing.T) {
	manifest := &Manifest{
		Phases: map[string]PhaseEntry{
			"0-preinstall": {Hash: "aaa", Completed: true},
		},
	}

	ctx1 := actions.NewBuildContext()
	ctx1.Partitions = []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 4 << 30, MountPoint: "/"},
	}

	ctx2 := actions.NewBuildContext()
	ctx2.Partitions = []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 4 << 30, MountPoint: "/"},
	}
	ctx2.InstallServer = &actions.InstallServerDef{Port: 8080, Path: "/opt/installer"}

	h1 := HashPackaging(manifest, ctx1, nil)
	h2 := HashPackaging(manifest, ctx2, nil)
	if h1 == h2 {
		t.Error("adding installer server should change packaging hash")
	}
}

func TestHashPhase_Files_Mkdir(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.FileMkdirs = []actions.FileMkdirOp{
		{Path: "/etc/custom", Mode: "0755", Owner: "root", Group: "root"},
	}
	h, err := HashPhase(4, ctx)
	if err != nil {
		t.Fatalf("HashPhase(4) error: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashPhase_Files_Create(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.FileCreates = []actions.FileCreateOp{
		{Path: "/etc/hostname", Mode: "0644", Content: "my-device"},
	}
	h, err := HashPhase(4, ctx)
	if err != nil {
		t.Fatalf("HashPhase(4) error: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashPhase_Files_DifferentContent_DifferentHash(t *testing.T) {
	ctx1 := actions.NewBuildContext()
	ctx1.FileCreates = []actions.FileCreateOp{{Path: "/etc/f", Content: "v1"}}

	ctx2 := actions.NewBuildContext()
	ctx2.FileCreates = []actions.FileCreateOp{{Path: "/etc/f", Content: "v2"}}

	h1, _ := HashPhase(4, ctx1)
	h2, _ := HashPhase(4, ctx2)
	if h1 == h2 {
		t.Error("different file content must produce different phase 4 hashes")
	}
}

func TestHashPhase_Files_Edit(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.FileEdits = []actions.FileEditOp{
		{Path: "/etc/fstab", Insert: "append", Content: "tmpfs /tmp tmpfs defaults 0 0"},
	}
	h, err := HashPhase(4, ctx)
	if err != nil {
		t.Fatalf("HashPhase(4) error: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashPhase_Files_LinksMovesDeletes(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.FileCopies = []actions.FileCopyOp{{FromPath: "/a", ToPath: "/b"}}
	ctx.FileMoves = []actions.FileMoveOp{{FromPath: "/c", ToPath: "/d"}}
	ctx.FileLinks = []actions.FileLinkOp{{ToPath: "/e", FromPath: "/f", Type: "symbolic"}}
	ctx.FileDeletes = []actions.FileDeleteOp{{Path: "/g", Recursive: true}}

	h, err := HashPhase(4, ctx)
	if err != nil {
		t.Fatalf("HashPhase(4) error: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashPhase_Permissions_Ownership(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.FileOwnerships = []actions.FileOwnershipOp{
		{Path: "/opt/app", Owner: "appuser", Group: "appgroup", Recursive: true},
	}
	h, err := HashPhase(5, ctx)
	if err != nil {
		t.Fatalf("HashPhase(5) error: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashPhase_Permissions_Mode(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.FilePermissions = []actions.FilePermissionOp{
		{Path: "/etc/shadow", Mode: "0600", Recursive: false},
	}
	h, err := HashPhase(5, ctx)
	if err != nil {
		t.Fatalf("HashPhase(5) error: %v", err)
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashPhase_Permissions_DifferentOwner_DifferentHash(t *testing.T) {
	ctx1 := actions.NewBuildContext()
	ctx1.FileOwnerships = []actions.FileOwnershipOp{{Path: "/data", Owner: "alice"}}

	ctx2 := actions.NewBuildContext()
	ctx2.FileOwnerships = []actions.FileOwnershipOp{{Path: "/data", Owner: "bob"}}

	h1, _ := HashPhase(5, ctx1)
	h2, _ := HashPhase(5, ctx2)
	if h1 == h2 {
		t.Error("different owners must produce different phase 5 hashes")
	}
}
