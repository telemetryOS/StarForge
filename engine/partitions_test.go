package engine

import (
	"testing"

	"github.com/telemetryos/starforge/actions"
)

func TestResolvePartitionSizes_NoDiskSize(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Size: 4 << 30, Grow: true},
	}
	result := ResolvePartitionSizes(parts, 0)
	if result[0].Size != 4<<30 {
		t.Errorf("diskSize=0: size = %d, want unchanged %d", result[0].Size, 4<<30)
	}
}

func TestResolvePartitionSizes_NoGrowable(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 512 << 20, Grow: false},
		{Name: "root", Size: 4 << 30, Grow: false},
	}
	result := ResolvePartitionSizes(parts, 10<<30)
	if result[0].Size != 512<<20 || result[1].Size != 4<<30 {
		t.Error("no-grow partitions must keep original sizes")
	}
}

func TestResolvePartitionSizes_SingleGrowable(t *testing.T) {
	// boot=512M fixed, root=2G+growable, disk=10G → root gets 2G + (10G-2.5G) = 9.5G
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 512 << 20, Grow: false},
		{Name: "root", Size: 2 << 30, Grow: true},
	}
	disk := uint64(10 << 30)
	result := ResolvePartitionSizes(parts, disk)
	fixed := uint64(512<<20) + (2 << 30)
	expected := (2 << 30) + (disk - fixed)
	if result[1].Size != expected {
		t.Errorf("single growable size = %d, want %d", result[1].Size, expected)
	}
}

func TestResolvePartitionSizes_MultipleGrowable(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "efi", Size: 512 << 20, Grow: false},
		{Name: "root", Size: 2 << 30, Grow: true},
		{Name: "home", Size: 1 << 30, Grow: true},
	}
	disk := uint64(12 << 30)
	result := ResolvePartitionSizes(parts, disk)
	fixed := uint64(512<<20) + (2 << 30) + (1 << 30)
	perGrow := (disk - fixed) / 2
	if result[1].Size != (2<<30)+perGrow {
		t.Errorf("root size = %d, want %d", result[1].Size, (2<<30)+perGrow)
	}
	if result[2].Size != (1<<30)+perGrow {
		t.Errorf("home size = %d, want %d", result[2].Size, (1<<30)+perGrow)
	}
}

func TestResolvePartitionSizes_DiskTooSmall(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Size: 10 << 30, Grow: true},
	}
	// disk < fixedTotal — function returns partitions unchanged
	result := ResolvePartitionSizes(parts, 5<<30)
	if result[0].Size != 10<<30 {
		t.Errorf("disk < fixed: size = %d, want unchanged %d", result[0].Size, 10<<30)
	}
}

func TestResolvePartitionSizes_ExactFit(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 512 << 20, Grow: false},
		{Name: "root", Size: 2 << 30, Grow: true},
	}
	disk := uint64(512<<20) + (2 << 30)
	result := ResolvePartitionSizes(parts, disk)
	// remaining = 0 → growable gets no extra
	if result[1].Size != 2<<30 {
		t.Errorf("exact fit: root size = %d, want %d", result[1].Size, 2<<30)
	}
}

func TestResolvePartitionSizes_EdgeOS_Layout(t *testing.T) {
	// Typical Edge-OS: efi=512M, recovery=6G, root=7G+, data=256M+
	parts := []actions.PartitionDef{
		{Name: "efi", Size: 512 << 20, Grow: false, Type: "efi"},
		{Name: "recovery", Size: 6 << 30, Grow: false, Type: "linux"},
		{Name: "root", Size: 7 << 30, Grow: true, Type: "root"},
		{Name: "data", Size: 256 << 20, Grow: true, Type: "linux"},
	}
	disk := uint64(32 << 30)
	result := ResolvePartitionSizes(parts, disk)

	fixed := uint64(512<<20) + (6 << 30) + (7 << 30) + (256 << 20)
	perGrow := (disk - fixed) / 2

	if result[2].Size != (7<<30)+perGrow {
		t.Errorf("root size = %d, want %d", result[2].Size, (7<<30)+perGrow)
	}
	if result[3].Size != (256<<20)+perGrow {
		t.Errorf("data size = %d, want %d", result[3].Size, (256<<20)+perGrow)
	}
}

func TestResolvePartitionSizes_PreservesInput(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Size: 4 << 30, Grow: true},
	}
	originalSize := parts[0].Size
	ResolvePartitionSizes(parts, 10<<30)
	if parts[0].Size != originalSize {
		t.Error("ResolvePartitionSizes must not modify the input slice")
	}
}

func TestResolvePartitionSizes_PercentageGrowable(t *testing.T) {
	// 100% partition: ParseSize("100%") returns Size=0, Grow=true
	parts := []actions.PartitionDef{
		{Name: "boot", Size: 512 << 20, Grow: false},
		{Name: "root", Size: 0, Grow: true}, // 100% — no minimum
	}
	disk := uint64(10 << 30)
	result := ResolvePartitionSizes(parts, disk)
	// fixed = 512M + 0 = 512M; remaining = 10G - 512M; root gets it all
	fixed := uint64(512 << 20)
	expected := disk - fixed
	if result[1].Size != expected {
		t.Errorf("100%% root size = %d, want %d", result[1].Size, expected)
	}
}
