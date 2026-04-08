package diskutil

import (
	"testing"
)

func TestFormatSize_Bytes(t *testing.T) {
	cases := []struct{ in uint64; want string }{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSize_Kilobytes(t *testing.T) {
	const KB = uint64(1024)
	cases := []struct{ in uint64; want string }{
		{KB, "1.0 KB"},
		{KB * 2, "2.0 KB"},
		{KB*3/2, "1.5 KB"}, // 1536 bytes = 1.5 KB
		{KB * 512, "512.0 KB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSize_Megabytes(t *testing.T) {
	const MB = uint64(1024 * 1024)
	cases := []struct{ in uint64; want string }{
		{MB, "1.0 MB"},
		{MB * 10, "10.0 MB"},
		{MB * 512, "512.0 MB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSize_Gigabytes(t *testing.T) {
	const GB = uint64(1024 * 1024 * 1024)
	cases := []struct{ in uint64; want string }{
		{GB, "1.0 GB"},
		{GB * 2, "2.0 GB"},
		{GB * 16, "16.0 GB"},
		{GB * 3 / 2, "1.5 GB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSize_Terabytes(t *testing.T) {
	const TB = uint64(1024 * 1024 * 1024 * 1024)
	cases := []struct{ in uint64; want string }{
		{TB, "1.0 TB"},
		{TB * 2, "2.0 TB"},
		{TB * 8, "8.0 TB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSize_UnitBoundaries(t *testing.T) {
	const (
		KB = uint64(1024)
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	// Just below boundary → smaller unit
	if got := FormatSize(KB - 1); got != "1023 B" {
		t.Errorf("FormatSize(KB-1) = %q, want %q", got, "1023 B")
	}
	if got := FormatSize(KB); got != "1.0 KB" {
		t.Errorf("FormatSize(KB) = %q, want %q", got, "1.0 KB")
	}
	if got := FormatSize(MB); got != "1.0 MB" {
		t.Errorf("FormatSize(MB) = %q, want %q", got, "1.0 MB")
	}
	if got := FormatSize(GB); got != "1.0 GB" {
		t.Errorf("FormatSize(GB) = %q, want %q", got, "1.0 GB")
	}
	if got := FormatSize(TB); got != "1.0 TB" {
		t.Errorf("FormatSize(TB) = %q, want %q", got, "1.0 TB")
	}
}
