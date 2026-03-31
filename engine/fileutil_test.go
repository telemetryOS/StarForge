package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// --- safeRootfsJoin tests ---

func TestSafeRootfsJoin_SafePaths(t *testing.T) {
	rootfs := "/rootfs"
	cases := []struct {
		path string
		want string
	}{
		{"/etc/hostname", "/rootfs/etc/hostname"},
		{"etc/hostname", "/rootfs/etc/hostname"},
		{"/boot/loader/entries/arch.conf", "/rootfs/boot/loader/entries/arch.conf"},
		{"/", "/rootfs"},
		{"", "/rootfs"},
		{"./etc/file", "/rootfs/etc/file"},
	}
	for _, c := range cases {
		got, err := safeRootfsJoin(rootfs, c.path)
		if err != nil {
			t.Errorf("safeRootfsJoin(%q) unexpected error: %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("safeRootfsJoin(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestSafeRootfsJoin_TraversalPaths(t *testing.T) {
	rootfs := "/rootfs"
	unsafe := []string{
		"../etc/passwd",
		"../../etc/shadow",
		"/etc/../../../etc/passwd",
		"etc/../../etc/passwd",
	}
	for _, p := range unsafe {
		_, err := safeRootfsJoin(rootfs, p)
		if err == nil {
			t.Errorf("safeRootfsJoin(%q) should have returned an error", p)
		}
	}
}

// --- writeFile tests ---

func TestWriteFile_CreatesFileWithContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.txt")

	if err := writeFile(path, "hello world"); err != nil {
		t.Fatalf("writeFile error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", string(data), "hello world")
	}
}

func TestWriteFile_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.txt")
	if err := writeFile(path, "data"); err != nil {
		t.Fatalf("writeFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b", "c")); err != nil {
		t.Errorf("parent dirs not created: %v", err)
	}
}

func TestWriteFile_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	writeFile(path, "original")
	if err := writeFile(path, "updated"); err != nil {
		t.Fatalf("writeFile error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "updated" {
		t.Errorf("content = %q, want %q", string(data), "updated")
	}
}

// --- appendFile tests ---

func TestAppendFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")
	if err := appendFile(path, "first"); err != nil {
		t.Fatalf("appendFile error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "first" {
		t.Errorf("content = %q, want %q", string(data), "first")
	}
}

func TestAppendFile_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	appendFile(path, "line1\n")
	appendFile(path, "line2\n")
	data, _ := os.ReadFile(path)
	if string(data) != "line1\nline2\n" {
		t.Errorf("content = %q, want %q", string(data), "line1\nline2\n")
	}
}

func TestAppendFile_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "file.log")
	if err := appendFile(path, "data"); err != nil {
		t.Fatalf("appendFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deep", "nested")); err != nil {
		t.Errorf("parent dirs not created: %v", err)
	}
}

// --- CopyFile tests ---

func TestCopyFile_CopiesContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("writing source: %v", err)
	}
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile error: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("copied content = %q, want %q", string(data), "hello")
	}
}

func TestCopyFile_ErrorOnMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := CopyFile(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "dst")); err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestCopyFile_IndependentContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")

	os.WriteFile(src, []byte{1, 2, 3}, 0o644)
	CopyFile(src, dst)

	// Modifying source after copy must not affect dest
	os.WriteFile(src, []byte{9, 9, 9}, 0o644)
	data, _ := os.ReadFile(dst)
	if len(data) != 3 || data[0] != 1 {
		t.Errorf("dest content changed after source was modified: %v", data)
	}
}
