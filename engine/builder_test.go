package engine

import (
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

// --- validateTargetHasRoot tests ---

func TestValidateTargetHasRoot_HasRoot(t *testing.T) {
	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", MountPoint: "/boot"},
			{Name: "root", MountPoint: "/"},
		},
	}
	if err := validateTargetHasRoot("device", ctx); err != nil {
		t.Errorf("validation should pass when / is declared: %v", err)
	}
}

func TestValidateTargetHasRoot_MissingRoot(t *testing.T) {
	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", MountPoint: "/boot"},
			{Name: "data", MountPoint: "/data"},
		},
	}
	err := validateTargetHasRoot("device", ctx)
	if err == nil {
		t.Fatal("expected error when no / partition declared")
	}
	if !strings.Contains(err.Error(), "no root (/) partition") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateTargetHasRoot_NoPartitions(t *testing.T) {
	ctx := &actions.BuildContext{Partitions: nil}
	if err := validateTargetHasRoot("device", ctx); err == nil {
		t.Error("expected error when no partitions declared at all")
	}
}

// --- buildRecursive cycle-detection tests ---
//
// These exercise just the cycle-check / visited-shortcircuit logic, both
// of which run before any filesystem work, so we don't need a real project.

func TestBuildRecursive_CycleDetected(t *testing.T) {
	b := &Builder{}
	visited := map[string]bool{}
	path := []string{"a", "b"}

	// Simulate: building "a" reached "b", which is now trying to build "a"
	// again. Should error before any other work.
	err := b.buildRecursive("a", config.Target{}, false, visited, path)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "a -> b -> a") {
		t.Errorf("expected full path in error, got: %v", err)
	}
}

func TestBuildRecursive_AlreadyVisitedShortCircuits(t *testing.T) {
	b := &Builder{}
	visited := map[string]bool{"x": true}
	if err := b.buildRecursive("x", config.Target{}, false, visited, nil); err != nil {
		t.Errorf("visited target should short-circuit cleanly, got: %v", err)
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
