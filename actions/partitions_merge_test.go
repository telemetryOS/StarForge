package actions

import "testing"

func TestMergePartitions_SingleContrib(t *testing.T) {
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
			{Name: "root", Filesystem: "ext4", Size: 12 << 30, Type: "linux", MountPoint: "/"},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Mounts["device"] != "/efi" {
		t.Errorf("boot mount = %q, want /efi", got[0].Mounts["device"])
	}
	if got[1].Mounts["device"] != "/" {
		t.Errorf("root mount = %q, want /", got[1].Mounts["device"])
	}
}

func TestMergePartitions_DedupeByName_KeepsPerTargetMounts(t *testing.T) {
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: nil},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
			{Name: "root-main", Filesystem: "ext4", Size: 12 << 30, Type: "linux", MountPoint: "/"},
			{Name: "root-recovery", Filesystem: "ext4", Size: 6 << 30, Type: "linux", MountPoint: "/recovery"},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
			{Name: "root-main", Filesystem: "ext4", Size: 12 << 30, Type: "linux", MountPoint: "/main"},
			{Name: "root-recovery", Filesystem: "ext4", Size: 6 << 30, Type: "linux", MountPoint: "/"},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	boot := got[0]
	if boot.Mounts["main"] != "/efi" || boot.Mounts["recovery"] != "/efi" {
		t.Errorf("boot mounts = %+v, want main=/efi recovery=/efi", boot.Mounts)
	}
	rootMain := got[1]
	if rootMain.Mounts["main"] != "/" || rootMain.Mounts["recovery"] != "/main" {
		t.Errorf("root-main mounts = %+v, want main=/ recovery=/main", rootMain.Mounts)
	}
	rootRecovery := got[2]
	if rootRecovery.Mounts["main"] != "/recovery" || rootRecovery.Mounts["recovery"] != "/" {
		t.Errorf("root-recovery mounts = %+v", rootRecovery.Mounts)
	}
}

func TestMergePartitions_TakesMaxSize(t *testing.T) {
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: nil},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 512 << 20, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Size != 1<<30 {
		t.Errorf("boot size = %d, want %d (max of contributions)", got[0].Size, 1<<30)
	}
}

func TestMergePartitions_FilesystemMismatch(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: nil},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "ext4", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err == nil {
		t.Fatal("expected filesystem-mismatch error")
	}
}

func TestMergePartitions_TypeMismatch(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: nil},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "xbootldr", MountPoint: "/efi"},
		}},
	})
	if err == nil {
		t.Fatal("expected type-mismatch error")
	}
}

func TestMergePartitions_HostSmallerThanEmbed_Errors(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 512 << 20, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err == nil {
		t.Fatal("expected size-too-small error when host < embed")
	}
}

func TestMergePartitions_HostLargerThanEmbed_OK(t *testing.T) {
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 2 << 30, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err != nil {
		t.Fatalf("host larger than embed should succeed: %v", err)
	}
	if got[0].Size != 2<<30 {
		t.Errorf("size = %d, want host's 2G", got[0].Size)
	}
}

func TestMergePartitions_HostSizeIgnoredWhenHostSilent(t *testing.T) {
	// Host doesn't declare 'boot' at all — embeds set the size, no error.
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: nil},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 4 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Size != 4<<30 {
		t.Errorf("size = %d, want max(embeds) = 4G", got[0].Size)
	}
}

func TestMergePartitions_GrowPropagates(t *testing.T) {
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: nil},
		{Target: "main", Parts: []PartitionDef{
			{Name: "data", Filesystem: "ext4", Size: 256 << 20, Type: "linux", MountPoint: "/data", Grow: false},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "data", Filesystem: "ext4", Size: 256 << 20, Type: "linux", MountPoint: "/data", Grow: true},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got[0].Grow {
		t.Error("grow should propagate when any contribution declares it")
	}
}

func TestMergePartitions_DuplicateInSameTarget(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err == nil {
		t.Fatal("expected error when one target declares the same partition twice")
	}
}

func TestMergePartitions_DuplicateTargetNameRejected(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "device", Parts: []PartitionDef{
			{Name: "root", Filesystem: "ext4", Size: 12 << 30, Type: "linux", MountPoint: "/"},
		}},
	})
	if err == nil {
		t.Fatal("duplicate target name across contribs should error")
	}
}

func TestMergePartitions_EmptyTargetNameRejected(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "", Parts: nil},
	})
	if err == nil {
		t.Fatal("empty target name should error")
	}
}

func TestMergePartitions_EmptyFilesystemRejected(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
	})
	if err == nil {
		t.Fatal("empty filesystem should error")
	}
}

func TestMergePartitions_EmptyTypeRejected(t *testing.T) {
	_, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "", MountPoint: "/efi"},
		}},
	})
	if err == nil {
		t.Fatal("empty type should error")
	}
}

func TestMergePartitions_OrderingHostFirstThenEmbeds(t *testing.T) {
	got, err := MergePartitions([]PartitionContribution{
		{Target: "device", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
		}},
		{Target: "main", Parts: []PartitionDef{
			{Name: "boot", Filesystem: "vfat", Size: 1 << 30, Type: "efi", MountPoint: "/efi"},
			{Name: "root", Filesystem: "ext4", Size: 12 << 30, Type: "linux", MountPoint: "/"},
		}},
		{Target: "recovery", Parts: []PartitionDef{
			{Name: "recovery", Filesystem: "ext4", Size: 6 << 30, Type: "linux", MountPoint: "/"},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"boot", "root", "recovery"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
}
