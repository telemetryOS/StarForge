package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

func initPhaseTest(t *testing.T) (dir string, cleanup func()) {
	t.Helper()
	dir = t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	return dir, func() { o.Close() }
}

func TestPhaseFiles_Mkdir(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	ctx := &actions.BuildContext{
		FileMkdirs: []actions.FileMkdirOp{
			{Path: "/etc/myapp", Mode: "0755"},
		},
	}
	b := &Builder{project: nil}
	if err := b.phaseFiles(ctx, rootfs); err != nil {
		t.Fatalf("phaseFiles error: %v", err)
	}

	info, err := os.Stat(filepath.Join(rootfs, "etc/myapp"))
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestPhaseFiles_FileCreate(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	ctx := &actions.BuildContext{
		FileCreates: []actions.FileCreateOp{
			{Path: "/etc/config.ini", Content: "key=value\n", Mode: "0644"},
		},
	}
	b := &Builder{project: nil}
	if err := b.phaseFiles(ctx, rootfs); err != nil {
		t.Fatalf("phaseFiles error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rootfs, "etc/config.ini"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "key=value\n" {
		t.Errorf("content = %q, want %q", string(data), "key=value\n")
	}
}

func TestPhaseFiles_FileDelete(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	target := filepath.Join(rootfs, "etc/old.conf")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("old"), 0o644)

	ctx := &actions.BuildContext{
		FileDeletes: []actions.FileDeleteOp{
			{Path: "/etc/old.conf", Recursive: false},
		},
	}
	b := &Builder{project: nil}
	if err := b.phaseFiles(ctx, rootfs); err != nil {
		t.Fatalf("phaseFiles error: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestPhaseFiles_FileDeleteRecursive(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	dir := filepath.Join(rootfs, "opt/old")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644)

	ctx := &actions.BuildContext{
		FileDeletes: []actions.FileDeleteOp{
			{Path: "/opt/old", Recursive: true},
		},
	}
	b := &Builder{project: nil}
	if err := b.phaseFiles(ctx, rootfs); err != nil {
		t.Fatalf("phaseFiles error: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should have been removed recursively")
	}
}

func TestPhaseFiles_SymlinkCreation(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755)

	ctx := &actions.BuildContext{
		FileLinks: []actions.FileLinkOp{
			{
				Type:     "symbolic",
				ToPath:   "/etc/link",
				FromPath: "/etc/target",
			},
		},
	}
	b := &Builder{project: nil}
	if err := b.phaseFiles(ctx, rootfs); err != nil {
		t.Fatalf("phaseFiles error: %v", err)
	}

	linkPath := filepath.Join(rootfs, "etc/link")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if target != "/etc/target" {
		t.Errorf("symlink target = %q, want %q", target, "/etc/target")
	}
}

func TestPhaseFiles_TraversalPathRejected(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	ctx := &actions.BuildContext{
		FileCreates: []actions.FileCreateOp{
			{Path: "../../etc/passwd", Content: "evil", Mode: "0644"},
		},
	}
	b := &Builder{project: nil}
	err := b.phaseFiles(ctx, rootfs)
	if err == nil {
		t.Fatal("expected error for path traversal in file-create")
	}
}

func TestPhaseFiles_EmptyContext_NoOp(t *testing.T) {
	rootfs, done := initPhaseTest(t)
	defer done()

	ctx := &actions.BuildContext{}
	b := &Builder{project: nil}
	if err := b.phaseFiles(ctx, rootfs); err != nil {
		t.Fatalf("phaseFiles on empty context: %v", err)
	}
}

// TestMountTable_SkipsEmptyMountPoint verifies that swap/no-mount partitions
// (MountPoint == "") are skipped rather than causing a mount at rootfs root.
func TestMountTable_SkipsEmptyMountPoint(t *testing.T) {
	rootfs := t.TempDir()
	mt := NewMountTable(rootfs)

	// Include a partition with empty MountPoint — it must be silently skipped.
	// (MountAll would fail trying to run the real mount command, but it should
	// never even attempt to mount the empty-MountPoint entry.)
	parts := []PartitionMount{
		{Source: "/dev/invalid", MountPoint: ""},     // swap — must be skipped
		{Source: "/dev/also-invalid", MountPoint: ""}, // another no-mount
	}

	// MountAll will fail because /dev/invalid doesn't exist, but ONLY if it
	// tries to mount it. If the empty check works, it returns no error (all
	// entries skipped).
	err := mt.MountAll(parts)
	if err != nil {
		t.Errorf("MountAll with only empty-MountPoint entries should skip all and succeed, got: %v", err)
	}

	if len(mt.mounts) != 0 {
		t.Errorf("expected 0 mounts recorded, got %d", len(mt.mounts))
	}
}
