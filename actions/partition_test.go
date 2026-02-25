package actions

import (
	"testing"
)

func TestRecordPartitionSnapshot(t *testing.T) {
	ctx := NewBuildContext()
	ctx.CurrentLayer = "layer-1"
	ctx.Partitions = []PartitionDef{
		{Name: "boot", Filesystem: "vfat", Size: 512 << 20},
		{Name: "root", Filesystem: "ext4", Size: 8 << 30},
	}

	ctx.RecordPartitionSnapshot()

	if len(ctx.PartitionHistory) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(ctx.PartitionHistory))
	}

	snap := ctx.PartitionHistory[0]
	if snap.Layer != "layer-1" {
		t.Errorf("snapshot layer = %q, want %q", snap.Layer, "layer-1")
	}
	if len(snap.Partitions) != 2 {
		t.Fatalf("snapshot has %d partitions, want 2", len(snap.Partitions))
	}
	if snap.Partitions[0].Name != "boot" || snap.Partitions[1].Name != "root" {
		t.Errorf("snapshot partitions = %+v", snap.Partitions)
	}

	// Verify deep copy: modifying ctx.Partitions should not affect the snapshot
	ctx.Partitions[0].Name = "modified"
	if snap.Partitions[0].Name != "boot" {
		t.Error("snapshot was not a deep copy — modifying ctx.Partitions changed the snapshot")
	}
}

func TestRecordPartitionSnapshot_Multiple(t *testing.T) {
	ctx := NewBuildContext()

	ctx.CurrentLayer = "base"
	ctx.Partitions = []PartitionDef{
		{Name: "root", Filesystem: "ext4", Size: 4 << 30},
	}
	ctx.RecordPartitionSnapshot()

	ctx.CurrentLayer = "overlay"
	ctx.Partitions = append(ctx.Partitions, PartitionDef{
		Name: "data", Filesystem: "ext4", Size: 1 << 30,
	})
	ctx.RecordPartitionSnapshot()

	if len(ctx.PartitionHistory) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(ctx.PartitionHistory))
	}

	// First snapshot should have 1 partition
	if len(ctx.PartitionHistory[0].Partitions) != 1 {
		t.Errorf("first snapshot has %d partitions, want 1", len(ctx.PartitionHistory[0].Partitions))
	}
	// Second snapshot should have 2 partitions
	if len(ctx.PartitionHistory[1].Partitions) != 2 {
		t.Errorf("second snapshot has %d partitions, want 2", len(ctx.PartitionHistory[1].Partitions))
	}
}
