package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// --- filterSwapEntries ---

func TestFilterSwapEntries_RemovesSwapLines(t *testing.T) {
	input := "UUID=abc / ext4 defaults 0 1\nUUID=def none swap sw 0 0\nUUID=ghi /boot vfat defaults 0 2\n"
	got := filterSwapEntries(input)
	if containsLine(got, "swap") {
		t.Errorf("swap line not removed:\n%s", got)
	}
	if !containsLine(got, "ext4") {
		t.Errorf("ext4 line was removed unexpectedly:\n%s", got)
	}
}

func TestFilterSwapEntries_NoSwap_Unchanged(t *testing.T) {
	input := "UUID=abc / ext4 defaults 0 1\nUUID=ghi /boot vfat defaults 0 2\n"
	got := filterSwapEntries(input)
	if got != input {
		t.Errorf("non-swap fstab should be unchanged\ngot:  %q\nwant: %q", got, input)
	}
}

func TestFilterSwapEntries_Empty(t *testing.T) {
	if got := filterSwapEntries(""); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
}

func TestFilterSwapEntries_OnlySwap(t *testing.T) {
	input := "UUID=def none swap sw 0 0"
	got := filterSwapEntries(input)
	if containsLine(got, "swap") {
		t.Errorf("swap line should be removed, got: %q", got)
	}
}

func TestFilterSwapEntries_CommentAndBlankLinesKept(t *testing.T) {
	input := "# fstab\n\nUUID=abc / ext4 defaults 0 1\n"
	got := filterSwapEntries(input)
	if !containsLine(got, "# fstab") {
		t.Errorf("comment line should be preserved:\n%s", got)
	}
}

// --- hasPartType ---

func TestHasPartType_MatchFound(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", Type: "efi"},
		{Name: "root", Type: "linux"},
	}
	if !hasPartType(parts, "efi") {
		t.Error("expected hasPartType=true for 'efi'")
	}
	if !hasPartType(parts, "linux") {
		t.Error("expected hasPartType=true for 'linux'")
	}
}

func TestHasPartType_NoMatch(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Type: "linux"},
	}
	if hasPartType(parts, "efi") {
		t.Error("expected hasPartType=false for 'efi'")
	}
}

func TestHasPartType_Empty(t *testing.T) {
	if hasPartType(nil, "efi") {
		t.Error("nil parts should return false")
	}
	if hasPartType([]actions.PartitionDef{}, "efi") {
		t.Error("empty parts should return false")
	}
}

// --- hasRootPartition ---

func TestHasRootPartition_Found(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", MountPoint: "/boot"},
		{Name: "root", MountPoint: "/"},
	}
	if !hasRootPartition(parts) {
		t.Error("expected hasRootPartition=true")
	}
}

func TestHasRootPartition_NotFound(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", MountPoint: "/boot"},
		{Name: "data", MountPoint: "/data"},
	}
	if hasRootPartition(parts) {
		t.Error("expected hasRootPartition=false")
	}
}

func TestHasRootPartition_Empty(t *testing.T) {
	if hasRootPartition(nil) {
		t.Error("nil should return false")
	}
}

// --- DescendantMountPaths ---

func TestDescendantMountPaths_BasicChildren(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", MountPoint: "/"},
		{Name: "boot", MountPoint: "/boot"},
		{Name: "data", MountPoint: "/var/data"},
	}
	got := DescendantMountPaths("/", parts)
	if len(got) != 2 {
		t.Fatalf("expected 2 descendants of /, got %d: %v", len(got), got)
	}
	wantSet := map[string]bool{"boot": true, "var/data": true}
	for _, p := range got {
		if !wantSet[p] {
			t.Errorf("unexpected path %q in descendants", p)
		}
	}
}

func TestDescendantMountPaths_NestedParent(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", MountPoint: "/"},
		{Name: "var", MountPoint: "/var"},
		{Name: "log", MountPoint: "/var/log"},
	}
	got := DescendantMountPaths("/var", parts)
	if len(got) != 1 || got[0] != "log" {
		t.Errorf("expected [log], got %v", got)
	}
}

func TestDescendantMountPaths_NoDescendants(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", MountPoint: "/"},
		{Name: "boot", MountPoint: "/boot"},
	}
	got := DescendantMountPaths("/boot", parts)
	if len(got) != 0 {
		t.Errorf("expected no descendants of /boot, got %v", got)
	}
}

func TestDescendantMountPaths_ExcludesSelf(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", MountPoint: "/"},
		{Name: "boot", MountPoint: "/boot"},
	}
	got := DescendantMountPaths("/", parts)
	for _, p := range got {
		if p == "." || p == "/" {
			t.Errorf("self should be excluded, got %q", p)
		}
	}
}

// --- EnsureChrootDirs ---

func TestEnsureChrootDirs_CreatesAllDirs(t *testing.T) {
	rootfs := t.TempDir()
	if err := EnsureChrootDirs(rootfs); err != nil {
		t.Fatalf("EnsureChrootDirs error: %v", err)
	}
	for _, dir := range []string{"proc", "sys", "dev", "run", "tmp"} {
		p := filepath.Join(rootfs, dir)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("directory %s not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestEnsureChrootDirs_Idempotent(t *testing.T) {
	rootfs := t.TempDir()
	EnsureChrootDirs(rootfs)
	if err := EnsureChrootDirs(rootfs); err != nil {
		t.Fatalf("EnsureChrootDirs second call error: %v", err)
	}
}

// --- CopyPartition ---
//
// Exercises last-writer-wins overwrite semantics. Depends on `tar` on $PATH.

func stageMergedFixture(t *testing.T, mountPoint string, srcContent map[string]string) string {
	t.Helper()
	mergedDir := t.TempDir()
	subdir := filepath.Join(mergedDir, mountPoint)
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir merged subtree: %v", err)
	}
	for relPath, content := range srcContent {
		full := filepath.Join(subdir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return mergedDir
}

func TestCopyPartition_OverwritesExistingFile(t *testing.T) {
	mergedDir := stageMergedFixture(t, "/boot", map[string]string{
		"loader/loader.conf": "new-version",
	})
	rootfs := t.TempDir()
	destLoader := filepath.Join(rootfs, "boot", "loader", "loader.conf")
	if err := os.MkdirAll(filepath.Dir(destLoader), 0o755); err != nil {
		t.Fatalf("mkdir dest parent: %v", err)
	}
	if err := os.WriteFile(destLoader, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale dest: %v", err)
	}

	part := actions.PartitionDef{
		Name:       "boot",
		Filesystem: "vfat",
		Type:       "efi",
		MountPoint: "/boot",
	}
	if err := CopyPartition(mergedDir, part, []actions.PartitionDef{part}, rootfs); err != nil {
		t.Fatalf("CopyPartition: %v", err)
	}

	got, err := os.ReadFile(destLoader)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "new-version" {
		t.Errorf("expected dest overwritten with new-version, got %q", got)
	}
}

// helper
func containsLine(s, substr string) bool {
	for _, line := range splitLines(s) {
		for i := 0; i <= len(line)-len(substr); i++ {
			if len(substr) == 0 || line[i:i+len(substr)] == substr {
				return true
			}
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
