package installations

import (
	"strings"
	"testing"

	"github.com/telemetryos/starforge/installer"
)

func TestValidateManifest_HappyPath(t *testing.T) {
	m := &installer.PayloadManifest{
		Name: "device",
		Partitions: []installer.PayloadPartition{
			{Name: "boot", Filesystem: "vfat", Type: "efi", MountPoint: "/efi", Artifact: "boot.img.corona"},
			{Name: "root", Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: "root.img.corona"},
		},
	}
	if err := validateManifest(m); err != nil {
		t.Errorf("happy path should pass: %v", err)
	}
}

func TestValidateManifest_RejectsTraversalMountPoint(t *testing.T) {
	cases := []string{"../etc", "/etc/../..", "/foo/../bar", "//double-slash", "relative", ""}
	for _, mp := range cases {
		if mp == "" {
			continue // empty is legal (swap)
		}
		m := &installer.PayloadManifest{
			Partitions: []installer.PayloadPartition{
				{Name: "x", Filesystem: "ext4", Type: "linux", MountPoint: mp, Artifact: "x.img.corona"},
			},
		}
		if err := validateManifest(m); err == nil {
			t.Errorf("mount_point %q should be rejected", mp)
		}
	}
}

func TestValidateManifest_RejectsKernelVfsTargets(t *testing.T) {
	for _, mp := range []string{"/proc", "/sys", "/dev", "/run"} {
		m := &installer.PayloadManifest{
			Partitions: []installer.PayloadPartition{
				{Name: "x", Filesystem: "ext4", Type: "linux", MountPoint: mp, Artifact: "x.img.corona"},
			},
		}
		if err := validateManifest(m); err == nil {
			t.Errorf("mount_point %q (kernel vfs) should be rejected", mp)
		}
	}
}

func TestValidateManifest_RejectsBadName(t *testing.T) {
	for _, name := range []string{"", "name with space", "../traversal", "name;rm", strings.Repeat("a", 37)} {
		m := &installer.PayloadManifest{
			Partitions: []installer.PayloadPartition{
				{Name: name, Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: "x.img.corona"},
			},
		}
		if err := validateManifest(m); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestValidateManifest_RejectsBadArtifact(t *testing.T) {
	for _, img := range []string{"../etc/passwd", "/abs/path", "sub/dir/img", "..", "."} {
		m := &installer.PayloadManifest{
			Partitions: []installer.PayloadPartition{
				{Name: "x", Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: img},
			},
		}
		if err := validateManifest(m); err == nil {
			t.Errorf("artifact %q should be rejected", img)
		}
	}
}

func TestValidateManifest_RejectsBadFilesystem(t *testing.T) {
	m := &installer.PayloadManifest{
		Partitions: []installer.PayloadPartition{
			{Name: "x", Filesystem: "ntfs-corrupt", Type: "linux", MountPoint: "/", Artifact: "x.img.corona"},
		},
	}
	if err := validateManifest(m); err == nil {
		t.Error("unsupported filesystem should be rejected")
	}
}

func TestValidateManifest_RejectsBadPartType(t *testing.T) {
	m := &installer.PayloadManifest{
		Partitions: []installer.PayloadPartition{
			{Name: "x", Filesystem: "ext4", Type: "made-up", MountPoint: "/", Artifact: "x.img.corona"},
		},
	}
	if err := validateManifest(m); err == nil {
		t.Error("unknown partition type should be rejected")
	}
}

func TestValidateManifest_RejectsDuplicateNames(t *testing.T) {
	m := &installer.PayloadManifest{
		Partitions: []installer.PayloadPartition{
			{Name: "x", Filesystem: "ext4", Type: "linux", MountPoint: "/a", Artifact: "x.img.corona"},
			{Name: "x", Filesystem: "ext4", Type: "linux", MountPoint: "/b", Artifact: "y.img.corona"},
		},
	}
	if err := validateManifest(m); err == nil {
		t.Error("duplicate partition name should be rejected")
	}
}

func TestValidateManifest_RejectsDuplicateMountPoints(t *testing.T) {
	m := &installer.PayloadManifest{
		Partitions: []installer.PayloadPartition{
			{Name: "a", Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: "a.img.corona"},
			{Name: "b", Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: "b.img.corona"},
		},
	}
	if err := validateManifest(m); err == nil {
		t.Error("duplicate mount_point should be rejected")
	}
}

func TestValidateManifest_AllowsEmptyMountPointForSwap(t *testing.T) {
	m := &installer.PayloadManifest{
		Partitions: []installer.PayloadPartition{
			{Name: "swap", Filesystem: "swap", Type: "swap", MountPoint: "", Artifact: ""},
			{Name: "root", Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: "root.img.corona"},
		},
	}
	if err := validateManifest(m); err != nil {
		t.Errorf("swap with empty mount_point + empty artifact should be legal: %v", err)
	}
}

func TestValidateManifest_RejectsEFILabelControlChars(t *testing.T) {
	m := &installer.PayloadManifest{
		EFILabel: "Edge\x1bOS",
		Partitions: []installer.PayloadPartition{
			{Name: "root", Filesystem: "ext4", Type: "linux", MountPoint: "/", Artifact: "x.img.corona"},
		},
	}
	if err := validateManifest(m); err == nil {
		t.Error("EFI label with escape sequence should be rejected")
	}
}

func TestValidateManifest_RejectsEmptyPartitions(t *testing.T) {
	if err := validateManifest(&installer.PayloadManifest{}); err == nil {
		t.Error("manifest with no partitions should be rejected")
	}
}
