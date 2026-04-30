package engine

import (
	"testing"

	"github.com/telemetryos/starforge/actions"
)

func TestSanitizeTargetName(t *testing.T) {
	cases := map[string]string{
		"main":          "main",
		"":              "_",
		".":             "_",
		"..":            "_",
		"../etc/passwd": "passwd",
		"safe_name":     "safe_name",
	}
	for in, want := range cases {
		if got := sanitizeTargetName(in); got != want {
			t.Errorf("sanitizeTargetName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMergedDiskPartitionsUseHostMountPoints(t *testing.T) {
	merged := []actions.MergedPartition{
		{
			Name:       "boot",
			Filesystem: "vfat",
			Size:       1 << 30,
			Type:       "efi",
			Mounts: map[string]string{
				"device":            "/efi",
				"fallback-recovery": "/boot",
			},
		},
		{
			Name:       "fallback-recovery",
			Filesystem: "ext4",
			Size:       6 << 30,
			Type:       "linux",
			Mounts: map[string]string{
				"device":            "/fallback-recovery",
				"fallback-recovery": "/",
			},
		},
	}

	got := mergedDiskPartitions("device", merged)
	if len(got) != 2 {
		t.Fatalf("mergedDiskPartitions returned %d partitions, want 2", len(got))
	}
	if got[0].Name != "boot" || got[0].MountPoint != "/efi" {
		t.Fatalf("boot host mount = %q, want /efi", got[0].MountPoint)
	}
	if got[1].Name != "fallback-recovery" || got[1].MountPoint != "/fallback-recovery" {
		t.Fatalf("fallback host mount = %q, want /fallback-recovery", got[1].MountPoint)
	}
}
