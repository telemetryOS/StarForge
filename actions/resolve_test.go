package actions

import (
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input   string
		bytes   uint64
		grow    bool
		wantErr bool
	}{
		// Fixed sizes
		{"512M", 512 << 20, false, false},
		{"1G", 1 << 30, false, false},
		{"4K", 4 << 10, false, false},
		{"1T", 1 << 40, false, false},
		{"100", 100, false, false},

		// Growable with minimum (Edge-OS uses 256M+ and 7G+)
		{"256M+", 256 << 20, true, false},
		{"7G+", 7 << 30, true, false},
		{"1T+", 1 << 40, true, false},

		// Percentage means growable, no minimum (Edge-OS uses 100%)
		{"100%", 0, true, false},

		// Whitespace trimmed
		{"  512M  ", 512 << 20, false, false},
		{" 256M+ ", 256 << 20, true, false},

		// Edge-OS exact values
		{"12G", 12 << 30, false, false}, // root partition
		{"6G", 6 << 30, false, false},   // recovery partitions

		// Errors
		{"", 0, false, true},
		{"abc", 0, false, true},
		{"M", 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			bytes, grow, err := ParseSize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSize(%q) expected error, got (%d, %v)", tt.input, bytes, grow)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSize(%q) unexpected error: %v", tt.input, err)
			}
			if bytes != tt.bytes {
				t.Errorf("ParseSize(%q) bytes = %d, want %d", tt.input, bytes, tt.bytes)
			}
			if grow != tt.grow {
				t.Errorf("ParseSize(%q) grow = %v, want %v", tt.input, grow, tt.grow)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{1 << 30, "1G"},
		{512 << 20, "512M"},
		{1 << 40, "1024G"}, // FormatSize only handles G/M/K, not T
		{4 << 10, "4K"},
		{12 << 30, "12G"},  // Edge-OS root partition
		{256 << 20, "256M"}, // Edge-OS data partition minimum
		{0, "0"},
		{1023, "1023"}, // non-aligned → raw digits
		{1<<20 + 1, "1048577"}, // not aligned to any unit
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestIsValidPartitionType(t *testing.T) {
	valid := []string{
		"linux", "efi", "swap", "home", "bios-boot",
		"raid", "lvm", "microsoft-basic", "microsoft-reserved",
		"root", "root-verity", "usr", "usr-verity",
	}
	for _, pt := range valid {
		if !isValidPartitionType(pt) {
			t.Errorf("isValidPartitionType(%q) = false, want true", pt)
		}
	}

	invalid := []string{"", "ext4", "LINUX", "Linux", "ntfs", "fat32"}
	for _, pt := range invalid {
		if isValidPartitionType(pt) {
			t.Errorf("isValidPartitionType(%q) = true, want false", pt)
		}
	}
}

func TestCopyPartitions(t *testing.T) {
	original := []PartitionDef{
		{Name: "boot", Filesystem: "vfat", Size: 1 << 30, MountPoint: "/boot", Type: "efi"},
		{Name: "root", Filesystem: "ext4", Size: 12 << 30, MountPoint: "/", Type: "linux"},
	}

	cp := copyPartitions(original)

	// Verify equal
	if len(cp) != len(original) {
		t.Fatalf("len(copy) = %d, want %d", len(cp), len(original))
	}
	for i := range original {
		if cp[i] != original[i] {
			t.Errorf("copy[%d] = %+v, want %+v", i, cp[i], original[i])
		}
	}

	// Modify copy, original must be unchanged
	cp[0].Name = "modified"
	cp[1].Size = 999
	if original[0].Name != "boot" {
		t.Error("modifying copy changed original[0].Name")
	}
	if original[1].Size != 12<<30 {
		t.Error("modifying copy changed original[1].Size")
	}
}
