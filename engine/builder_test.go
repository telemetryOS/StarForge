package engine

import (
	"strings"
	"testing"
)

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
