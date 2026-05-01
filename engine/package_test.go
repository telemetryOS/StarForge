package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// --- buildFstab ---

func TestBuildFstab_GeneratesDeclaredMountsOnly(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", Filesystem: "fat32", MountPoint: "/boot"},
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
		{Name: "swap", Filesystem: "swap"},
		{Name: "data", Filesystem: "ext4", MountPoint: "/data"},
		{Name: "payload", Filesystem: "ext4"},
	}
	info := map[string]fstabMountInfo{
		"/mnt/root":      {Source: "/dev/loop2", FSType: "ext4", UUID: "root-uuid"},
		"/mnt/root/boot": {Source: "/dev/loop3", FSType: "vfat", UUID: "boot-uuid"},
		"/mnt/root/data": {Source: "/dev/loop4", FSType: "ext4", UUID: "data-uuid"},
	}

	got, err := buildFstab(parts, "/mnt/root", func(target string) (fstabMountInfo, error) {
		mountInfo, ok := info[target]
		if !ok {
			t.Fatalf("unexpected fstab lookup for %q", target)
		}
		return mountInfo, nil
	})
	if err != nil {
		t.Fatalf("buildFstab failed: %v", err)
	}

	want := "UUID=root-uuid\t/\text4\trw,relatime\t0 1\n" +
		"UUID=boot-uuid\t/boot\tvfat\trw,relatime,fmask=0022,dmask=0022,codepage=437,iocharset=ascii,shortname=mixed,utf8,errors=remount-ro\t0 0\n" +
		"UUID=data-uuid\t/data\text4\trw,relatime\t0 2"
	if got != want {
		t.Errorf("unexpected fstab\ngot:\n%s\nwant:\n%s", got, want)
	}
	if containsLine(got, "swap") || containsLine(got, "/dev/loop") {
		t.Errorf("swap or build device source leaked into fstab:\n%s", got)
	}
}

func TestBuildFstab_FallsBackToDeclaredFilesystem(t *testing.T) {
	parts := []actions.PartitionDef{{Name: "boot", Filesystem: "fat32", MountPoint: "/boot"}}
	got, err := buildFstab(parts, "/mnt/root", func(target string) (fstabMountInfo, error) {
		return fstabMountInfo{UUID: "boot-uuid"}, nil
	})
	if err != nil {
		t.Fatalf("buildFstab failed: %v", err)
	}
	if !containsLine(got, "\t/boot\tvfat\t") {
		t.Errorf("fat32 filesystem was not normalized to vfat:\n%s", got)
	}
}

func TestBuildFstab_RequiresUUID(t *testing.T) {
	parts := []actions.PartitionDef{{Name: "root", Filesystem: "ext4", MountPoint: "/"}}
	_, err := buildFstab(parts, "/mnt/root", func(target string) (fstabMountInfo, error) {
		return fstabMountInfo{FSType: "ext4"}, nil
	})
	if err == nil {
		t.Fatal("expected missing UUID to fail")
	}
}

func TestBuildFstab_SortsParentsBeforeChildren(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "log", Filesystem: "ext4", MountPoint: "/var/log"},
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
		{Name: "var", Filesystem: "ext4", MountPoint: "/var"},
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
	}
	uuids := map[string]string{
		"/mnt/root":         "root-uuid",
		"/mnt/root/boot":    "boot-uuid",
		"/mnt/root/var":     "var-uuid",
		"/mnt/root/var/log": "log-uuid",
	}

	got, err := buildFstab(parts, "/mnt/root", func(target string) (fstabMountInfo, error) {
		return fstabMountInfo{FSType: "ext4", UUID: uuids[target]}, nil
	})
	if err != nil {
		t.Fatalf("buildFstab failed: %v", err)
	}

	assertLineOrder(t, got, "\t/\t", "\t/boot\t", "\t/var\t", "\t/var/log\t")
}

func TestBuildFstab_PropagatesMountLookupError(t *testing.T) {
	wantErr := errors.New("not mounted")
	parts := []actions.PartitionDef{{Name: "root", Filesystem: "ext4", MountPoint: "/"}}

	_, err := buildFstab(parts, "/mnt/root", func(target string) (fstabMountInfo, error) {
		return fstabMountInfo{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected lookup error %v, got %v", wantErr, err)
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

func TestPartitionBelongsToDevice(t *testing.T) {
	tests := []struct {
		source string
		device string
		want   bool
	}{
		{"/dev/sda1", "/dev/sda", true},
		{"/dev/nvme0n1p2", "/dev/nvme0n1", true},
		{"/dev/mmcblk0p3", "/dev/mmcblk0", true},
		{"/dev/sdaa1", "/dev/sda", false},
		{"/dev/sda", "/dev/sda", false},
	}
	for _, tt := range tests {
		if got := partitionBelongsToDevice(tt.source, tt.device); got != tt.want {
			t.Errorf("partitionBelongsToDevice(%q, %q) = %v, want %v", tt.source, tt.device, got, tt.want)
		}
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

func assertLineOrder(t *testing.T, s string, substrs ...string) {
	t.Helper()
	last := -1
	for _, substr := range substrs {
		idx := strings.Index(s, substr)
		if idx == -1 {
			t.Fatalf("expected %q in:\n%s", substr, s)
		}
		if idx < last {
			t.Fatalf("expected %q after previous mount in:\n%s", substr, s)
		}
		last = idx
	}
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
