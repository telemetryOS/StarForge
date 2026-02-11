package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// --- matchesAnyGroup tests ---

func TestMatchesAnyGroup(t *testing.T) {
	tests := []struct {
		pkgGroups []string
		requested []string
		want      bool
	}{
		{[]string{"build"}, []string{"build"}, true},
		{[]string{"build"}, []string{"run"}, false},
		{[]string{"build", "run"}, []string{"run"}, true},
		{[]string{"build", "run"}, []string{"build"}, true},
		{[]string{"build"}, []string{"build", "run"}, true},
		{[]string{"run"}, []string{"build"}, false},
		{nil, []string{"build"}, false},
		{[]string{"build"}, nil, false},
	}
	for _, tt := range tests {
		got := matchesAnyGroup(tt.pkgGroups, tt.requested)
		if got != tt.want {
			t.Errorf("matchesAnyGroup(%v, %v) = %v, want %v",
				tt.pkgGroups, tt.requested, got, tt.want)
		}
	}
}

// --- containsGroup tests ---

func TestContainsGroup(t *testing.T) {
	tests := []struct {
		groups []string
		group  string
		want   bool
	}{
		{[]string{"build", "run"}, "build", true},
		{[]string{"build", "run"}, "run", true},
		{[]string{"build"}, "run", false},
		{nil, "build", false},
		{[]string{}, "build", false},
	}
	for _, tt := range tests {
		got := containsGroup(tt.groups, tt.group)
		if got != tt.want {
			t.Errorf("containsGroup(%v, %q) = %v, want %v",
				tt.groups, tt.group, got, tt.want)
		}
	}
}

// --- patchPacstrap tests ---

func TestPatchPacstrap(t *testing.T) {
	dir := t.TempDir()

	// Create a fake pacstrap script
	script := "#!/bin/bash\n# original pacstrap script content\necho hello\n"
	os.WriteFile(filepath.Join(dir, "pacstrap"), []byte(script), 0o755)

	patchPacstrap(dir)

	data, err := os.ReadFile(filepath.Join(dir, "pacstrap"))
	if err != nil {
		t.Fatalf("reading patched script: %v", err)
	}
	content := string(data)

	// Should contain the marker
	if got := content; !containsStr(got, "# starforge-patched") {
		t.Error("patched script should contain marker")
	}

	// Should contain PATH export
	if !containsStr(content, "export PATH=") {
		t.Error("patched script should contain PATH export")
	}

	// Patching again should be a no-op (idempotent)
	patchPacstrap(dir)
	data2, _ := os.ReadFile(filepath.Join(dir, "pacstrap"))
	if string(data) != string(data2) {
		t.Error("second patch should be idempotent")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- ResolvePartitionSizes tests ---

func TestResolvePartitionSizes_NoGrow(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 512 << 20},
		{Name: "root", Size: 12 << 30},
	}
	result := ResolvePartitionSizes(parts, 32<<30)
	// No growable partitions, sizes unchanged
	if result[0].Size != 512<<20 {
		t.Errorf("boot size = %d, want %d", result[0].Size, 512<<20)
	}
	if result[1].Size != 12<<30 {
		t.Errorf("root size = %d, want %d", result[1].Size, 12<<30)
	}
}

func TestResolvePartitionSizes_ZeroDisk(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Size: 1 << 30, Grow: true},
	}
	result := ResolvePartitionSizes(parts, 0)
	// Zero disk size, return as-is
	if result[0].Size != 1<<30 {
		t.Errorf("root size = %d, want %d", result[0].Size, 1<<30)
	}
}

func TestResolvePartitionSizes_SingleGrow(t *testing.T) {
	// Edge-OS pattern: boot=512M fixed, root=7G+ growable
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 512 << 20, Grow: false},
		{Name: "root", Size: 7 << 30, Grow: true},
	}
	diskSize := uint64(32 << 30) // 32G disk
	result := ResolvePartitionSizes(parts, diskSize)

	// boot stays at 512M
	if result[0].Size != 512<<20 {
		t.Errorf("boot size = %d, want %d", result[0].Size, 512<<20)
	}
	// root should grow: 7G + remaining
	fixedTotal := uint64(512<<20) + uint64(7<<30)
	remaining := diskSize - fixedTotal
	expectedRoot := uint64(7<<30) + remaining
	if result[1].Size != expectedRoot {
		t.Errorf("root size = %d, want %d", result[1].Size, expectedRoot)
	}
}

func TestResolvePartitionSizes_MultipleGrow(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 256 << 20, Grow: false},
		{Name: "root", Size: 0, Grow: true},   // 100% growable
		{Name: "data", Size: 256 << 20, Grow: true}, // 256M+ growable
	}
	diskSize := uint64(16 << 30) // 16G
	result := ResolvePartitionSizes(parts, diskSize)

	// Remaining after fixed+minimum: 16G - (256M + 0 + 256M) = ~15.5G
	// Split equally between 2 growable partitions
	fixedTotal := uint64(256<<20) + 0 + uint64(256<<20)
	remaining := diskSize - fixedTotal
	perGrow := remaining / 2

	if result[1].Size != perGrow {
		t.Errorf("root size = %d, want %d", result[1].Size, perGrow)
	}
	expectedData := uint64(256<<20) + perGrow
	if result[2].Size != expectedData {
		t.Errorf("data size = %d, want %d", result[2].Size, expectedData)
	}
}

func TestResolvePartitionSizes_DiskSmallerThanFixed(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Size: 20 << 30, Grow: true},
	}
	// Disk is smaller than the fixed total
	result := ResolvePartitionSizes(parts, 10<<30)
	// Should return unchanged since diskSize <= fixedTotal
	if result[0].Size != 20<<30 {
		t.Errorf("root size = %d, want unchanged %d", result[0].Size, 20<<30)
	}
}

func TestResolvePartitionSizes_EdgeOS_FullLayout(t *testing.T) {
	// Simulate Edge-OS partition layout: boot=1G, root-a=12G, root-b=6G, recovery-a=6G, recovery-b=6G, data=256M+
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 1 << 30, Type: "efi", Filesystem: "vfat", MountPoint: "/boot", Grow: false},
		{Name: "root-a", Size: 12 << 30, Type: "linux", Filesystem: "ext4", MountPoint: "/", Grow: false},
		{Name: "root-b", Size: 6 << 30, Type: "linux", Filesystem: "ext4", Grow: false},
		{Name: "recovery-a", Size: 6 << 30, Type: "linux", Filesystem: "ext4", Grow: false},
		{Name: "recovery-b", Size: 6 << 30, Type: "linux", Filesystem: "ext4", Grow: false},
		{Name: "data", Size: 256 << 20, Type: "linux", Filesystem: "ext4", MountPoint: "/data", Grow: true},
	}

	// 64G disk
	diskSize := uint64(64 << 30)
	result := ResolvePartitionSizes(parts, diskSize)

	// Fixed total: 1G + 12G + 6G + 6G + 6G + 256M
	fixedTotal := uint64(1<<30) + uint64(12<<30) + uint64(6<<30)*3 + uint64(256<<20)
	remaining := diskSize - fixedTotal

	// Only data is growable, gets all remaining
	expectedData := uint64(256<<20) + remaining
	if result[5].Size != expectedData {
		t.Errorf("data partition size = %d, want %d", result[5].Size, expectedData)
	}

	// All non-growable partitions should be unchanged
	if result[0].Size != 1<<30 {
		t.Errorf("boot size changed")
	}
	if result[1].Size != 12<<30 {
		t.Errorf("root-a size changed")
	}
}

// --- MountTable tests ---

func TestNewMountTable(t *testing.T) {
	mt := NewMountTable("/tmp/test-rootfs")
	if mt.Rootfs() != "/tmp/test-rootfs" {
		t.Errorf("Rootfs() = %q, want %q", mt.Rootfs(), "/tmp/test-rootfs")
	}
}
