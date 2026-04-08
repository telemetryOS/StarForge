package engine

import (
	"os"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

// --- parseMode tests ---

func TestParseMode(t *testing.T) {
	tests := []struct {
		input      string
		defaultMod os.FileMode
		want       os.FileMode
	}{
		{"0644", 0o755, 0o644},
		{"0755", 0o644, 0o755},
		{"0600", 0o644, 0o600},
		{"0777", 0o644, 0o777},
		{"0444", 0o644, 0o444},
		{"", 0o644, 0o644}, // empty returns default
	}
	for _, tt := range tests {
		got, err := parseMode(tt.input, tt.defaultMod)
		if err != nil {
			t.Errorf("parseMode(%q, %o) unexpected error: %v", tt.input, tt.defaultMod, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMode(%q, %o) = %o, want %o", tt.input, tt.defaultMod, got, tt.want)
		}
	}
}

func TestParseMode_Error(t *testing.T) {
	invalids := []string{"invalid", "banana", "999", "rwxr-xr-x"}
	for _, input := range invalids {
		_, err := parseMode(input, 0o644)
		if err == nil {
			t.Errorf("parseMode(%q, 0644) should return error for non-empty invalid mode", input)
		}
	}
}

// --- filesystemForPath tests ---

func TestFilesystemForPath_Default(t *testing.T) {
	// No partitions → default ext4
	got := filesystemForPath("/etc/hostname", nil)
	if got != "ext4" {
		t.Errorf("filesystemForPath with nil parts = %q, want %q", got, "ext4")
	}
}

func TestFilesystemForPath_RootPartition(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
	}
	got := filesystemForPath("/etc/hostname", parts)
	if got != "ext4" {
		t.Errorf("got %q, want %q", got, "ext4")
	}
}

func TestFilesystemForPath_BootPartition(t *testing.T) {
	// Edge-OS layout: boot=vfat, root=ext4
	parts := []actions.PartitionDef{
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
	}
	got := filesystemForPath("/boot/EFI/BOOT/BOOTX64.EFI", parts)
	if got != "vfat" {
		t.Errorf("got %q, want %q", got, "vfat")
	}
}

func TestFilesystemForPath_MostSpecificMatch(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
		{Name: "data", Filesystem: "btrfs", MountPoint: "/var/data"},
	}
	// /var/data/file should match /var/data (more specific than /)
	got := filesystemForPath("/var/data/myfile.db", parts)
	if got != "btrfs" {
		t.Errorf("got %q, want %q", got, "btrfs")
	}

	// /var/log should match / (no /var partition)
	got = filesystemForPath("/var/log/syslog", parts)
	if got != "ext4" {
		t.Errorf("got %q, want %q", got, "ext4")
	}
}

func TestFilesystemForPath_ExactMountPoint(t *testing.T) {
	parts := []actions.PartitionDef{
		{Name: "root", Filesystem: "ext4", MountPoint: "/"},
		{Name: "boot", Filesystem: "vfat", MountPoint: "/boot"},
	}
	got := filesystemForPath("/boot", parts)
	if got != "vfat" {
		t.Errorf("got %q, want %q", got, "vfat")
	}
}

// --- boolToNo tests ---

func TestBoolToNo(t *testing.T) {
	if boolToNo(true) != "yes" {
		t.Errorf("boolToNo(true) = %q, want %q", boolToNo(true), "yes")
	}
	if boolToNo(false) != "no" {
		t.Errorf("boolToNo(false) = %q, want %q", boolToNo(false), "no")
	}
}

// --- phaseBoot .conf extension tests ---

func TestPhaseBoot_EntryNameGetsConfExtension(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{
		Boot: &actions.BootConfig{
			Loader: config.BootLoader{
				Default: "arch",
				Timeout: 3,
			},
			Entries: []config.BootEntry{
				{
					Name:    "arch", // no .conf extension
					Title:   "Arch Linux",
					Linux:   "/vmlinuz-linux",
					Initrd:  "/initramfs-linux.img",
					Options: "root=/dev/sda2 rw",
				},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Fatalf("phaseBoot returned error: %v", err)
	}

	// Verify entry written with .conf suffix
	entryPath := dir + "/boot/loader/entries/arch.conf"
	if _, err := os.Stat(entryPath); err != nil {
		t.Errorf("expected entry at %s, got: %v", entryPath, err)
	}
}

func TestPhaseBoot_EntryNameAlreadyHasConf(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{
		Boot: &actions.BootConfig{
			Loader: config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				{
					Name:    "arch.conf", // already has .conf
					Title:   "Arch Linux",
					Linux:   "/vmlinuz-linux",
					Initrd:  "/initramfs-linux.img",
					Options: "root=/dev/sda2 rw",
				},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Fatalf("phaseBoot returned error: %v", err)
	}

	// Verify entry is not double-suffixed
	entryPath := dir + "/boot/loader/entries/arch.conf"
	if _, err := os.Stat(entryPath); err != nil {
		t.Errorf("expected entry at %s, got: %v", entryPath, err)
	}
	// Must not exist with double suffix
	doubled := dir + "/boot/loader/entries/arch.conf.conf"
	if _, err := os.Stat(doubled); err == nil {
		t.Errorf("entry should not exist with double .conf: %s", doubled)
	}
}
