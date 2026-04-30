package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

func TestFilterPartitions_EmptyWantReturnsAll(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot"}, {Name: "root"}, {Name: "data"},
	}
	got := filterPartitions(parts, nil)
	if len(got) != len(parts) {
		t.Errorf("filterPartitions(nil) = %d, want %d (all)", len(got), len(parts))
	}
}

func TestFilterPartitions_MatchesByName(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot"}, {Name: "xbootldr"}, {Name: "root"}, {Name: "data"},
	}
	got := filterPartitions(parts, []string{"root", "xbootldr"})
	if len(got) != 2 {
		t.Fatalf("filterPartitions = %d, want 2", len(got))
	}
	// Order follows parts (layout order), not want.
	if got[0].Name != "xbootldr" || got[1].Name != "root" {
		t.Errorf("filterPartitions order = [%s %s], want [xbootldr root]", got[0].Name, got[1].Name)
	}
}

func TestFilterPartitions_UnknownNameSkipped(t *testing.T) {
	parts := []actions.PartitionDef{{Name: "root"}}
	got := filterPartitions(parts, []string{"root", "missing"})
	if len(got) != 1 || got[0].Name != "root" {
		t.Errorf("filterPartitions should skip unknown names: %v", got)
	}
}

func TestFirstMissingImage_AllPresent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"boot", "root"} {
		if err := os.WriteFile(filepath.Join(dir, name+".img"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	parts := []actions.PartitionDef{{Name: "boot"}, {Name: "root"}}
	if got := firstMissingImage(dir, parts); got != "" {
		t.Errorf("firstMissingImage = %q, want empty (all present)", got)
	}
}

func TestFirstMissingImage_ReportsFirstMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boot.img"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// root.img not created.
	parts := []actions.PartitionDef{{Name: "boot"}, {Name: "root"}}
	if got := firstMissingImage(dir, parts); got != "root.img" {
		t.Errorf("firstMissingImage = %q, want root.img", got)
	}
}
