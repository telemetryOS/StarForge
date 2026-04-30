package engine

import (
	"os"
	"strings"
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

// --- phaseBoot entry routing tests ---

// makeKernelOnBoot creates a fake kernel + initrd at <dir>/boot/ — the
// canonical pacman destination for kernel files in the test fixtures.
func makeKernelOnBoot(t *testing.T, dir, kernel string) {
	t.Helper()
	makeKernelAt(t, dir, "/boot", kernel)
}

// makeKernelAt creates a fake kernel + initrd at <dir><stagePath>/, used to
// pre-stage boot files when the entry's mount point isn't /boot.
func makeKernelAt(t *testing.T, dir, stagePath, kernel string) {
	t.Helper()
	stageDir := dir + stagePath
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", stagePath, err)
	}
	if err := os.WriteFile(stageDir+"/vmlinuz-"+kernel, []byte("kernel"), 0o644); err != nil {
		t.Fatalf("write vmlinuz: %v", err)
	}
	if err := os.WriteFile(stageDir+"/initramfs-"+kernel+".img", []byte("initrd"), 0o644); err != nil {
		t.Fatalf("write initrd: %v", err)
	}
}

func TestPhaseBoot_DefaultsToEspWhenNoXbootldr(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				{Name: "arch.conf", Title: "Arch", Kernel: "linux", Options: "rw"},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Fatalf("phaseBoot returned error: %v", err)
	}

	if _, err := os.Stat(dir + "/boot/loader/entries/arch.conf"); err != nil {
		t.Errorf("expected entry under /boot (ESP only): %v", err)
	}
}

func TestPhaseBoot_WritesSortKey(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "tos-*", Timeout: 0},
			Entries: []config.BootEntry{
				{Name: "tos-0-arch+3-0.conf", Title: "Arch", SortKey: "tos-0", Kernel: "linux", Options: "rw"},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Fatalf("phaseBoot returned error: %v", err)
	}
	data, err := os.ReadFile(dir + "/boot/loader/entries/tos-0-arch+3-0.conf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "sort-key tos-0\n") {
		t.Fatalf("entry missing sort-key:\n%s", string(data))
	}
}

func TestPhaseBoot_DefaultsToXbootldrWhenPresent(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/efi"},
			{Name: "xbootldr", Type: "xbootldr", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				{Name: "arch.conf", Title: "Arch", Kernel: "linux", Options: "rw"},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Fatalf("phaseBoot returned error: %v", err)
	}

	if _, err := os.Stat(dir + "/boot/loader/entries/arch.conf"); err != nil {
		t.Errorf("expected entry under XBOOTLDR /boot: %v", err)
	}
}

func TestPhaseBoot_ExplicitFalseGoesToEsp(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	// With auto-staging removed, the kernel/initrd must already exist at
	// the entry's destination. The OS layer is responsible for placing
	// them there (e.g. via a file-copy action or by mounting the partition
	// at /boot so pacstrap writes there directly). Pre-stage them by hand
	// for this routing test.
	makeKernelAt(t, dir, "/efi", "linux")

	false_ := false
	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/efi"},
			{Name: "xbootldr", Type: "xbootldr", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				{Name: "fallback.conf", Title: "Fallback", Kernel: "linux", Options: "rw", Extended: &false_},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Fatalf("phaseBoot returned error: %v", err)
	}

	if _, err := os.Stat(dir + "/efi/loader/entries/fallback.conf"); err != nil {
		t.Errorf("expected entry under /efi: %v", err)
	}
}

func TestPhaseBoot_ExtendedTrueWithoutXbootldrErrors(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	true_ := true
	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				{Name: "arch.conf", Title: "Arch", Kernel: "linux", Options: "rw", Extended: &true_},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err == nil {
		t.Error("expected error when extended=true but no XBOOTLDR partition declared")
	}
}

func TestPhaseBoot_MissingKernelAtEntryDestErrors(t *testing.T) {
	// The engine no longer auto-copies kernels between mount points.
	// If the entry's destination doesn't already have the kernel/initrd
	// (because the OS layer didn't arrange for pacman/file-copy to put
	// them there), phase_boot must fail with a clear error.
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	// Deliberately do NOT make a kernel.

	false_ := false
	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/efi"},
			{Name: "xbootldr", Type: "xbootldr", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Entries: []config.BootEntry{
				{Name: "fallback.conf", Title: "Fallback", Kernel: "linux", Extended: &false_, Options: "rw"},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err == nil {
		t.Error("expected error when kernel file is missing at the entry destination")
	}
}

func TestPhaseBoot_KernelPresentAtCanonicalDestSucceeds(t *testing.T) {
	// When an entry on XBOOTLDR (mount /boot) names a kernel that this
	// target's pacstrap wrote at /boot/vmlinuz-<kernel>, phase_boot writes
	// the entry without copying anything. Verify that path succeeds.
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/efi"},
			{Name: "xbootldr", Type: "xbootldr", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Entries: []config.BootEntry{
				{Name: "shared.conf", Title: "Shared", Kernel: "linux", Options: "rw"},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err != nil {
		t.Errorf("phase_boot should succeed when kernel is present at the canonical /boot path; got: %v", err)
	}
	// Entry .conf should still be written.
	if _, err := os.Stat(dir + "/boot/loader/entries/shared.conf"); err != nil {
		t.Errorf("entry should be written: %v", err)
	}
}

func TestPhaseBoot_PathOutsidePartitionRejected(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/efi"},
			{Name: "xbootldr", Type: "xbootldr", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				// Default extended=true (XBOOTLDR/boot), but path is under /efi → invalid
				{Name: "x.conf", Title: "X", Kernel: "linux", Path: "/efi/oops", Options: "rw"},
			},
		},
	}

	b := &Builder{project: nil}
	if err := b.phaseBoot(ctx, dir); err == nil {
		t.Error("expected error when path is outside the entry's partition mount")
	}
}

// --- phaseBoot .conf extension tests ---

func TestPhaseBoot_EntryNameGetsConfExtension(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{
				Default: "arch",
				Timeout: 3,
			},
			Entries: []config.BootEntry{
				{
					Name:    "arch", // no .conf extension
					Title:   "Arch Linux",
					Kernel:  "linux",
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
	makeKernelOnBoot(t, dir, "linux")

	ctx := &actions.BuildContext{
		Partitions: []actions.PartitionDef{
			{Name: "boot", Type: "efi", MountPoint: "/boot"},
		},
		Boot: &actions.BootConfig{
			Loader: &config.BootLoader{Default: "arch", Timeout: 3},
			Entries: []config.BootEntry{
				{
					Name:    "arch.conf", // already has .conf
					Title:   "Arch Linux",
					Kernel:  "linux",
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
