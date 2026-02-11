package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/telemetryos/starforge/config"
)

// FormatSize formats a byte count as a human-readable string (e.g. "512M", "4G").
func FormatSize(bytes uint64) string {
	switch {
	case bytes >= 1<<30 && bytes%(1<<30) == 0:
		return fmt.Sprintf("%dG", bytes/(1<<30))
	case bytes >= 1<<20 && bytes%(1<<20) == 0:
		return fmt.Sprintf("%dM", bytes/(1<<20))
	case bytes >= 1<<10 && bytes%(1<<10) == 0:
		return fmt.Sprintf("%dK", bytes/(1<<10))
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// ParseSize parses a size string like "512M", "4G", "100%".
// Returns (bytes, grow, error). grow is true for percentage values or "+" suffix.
func ParseSize(s string) (uint64, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, fmt.Errorf("size is required")
	}

	// Percentage means growable (no minimum size)
	if strings.HasSuffix(s, "%") {
		return 0, true, nil
	}

	// "+" suffix means growable with a minimum size (e.g. "256M+")
	grow := false
	if strings.HasSuffix(s, "+") {
		grow = true
		s = s[:len(s)-1]
	}

	multiplier := uint64(1)
	numStr := s

	switch {
	case strings.HasSuffix(s, "T"):
		multiplier = 1 << 40
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "G"):
		multiplier = 1 << 30
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "M"):
		multiplier = 1 << 20
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "K"):
		multiplier = 1 << 10
		numStr = s[:len(s)-1]
	}

	val, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid size %q: %w", s, err)
	}

	return val * multiplier, grow, nil
}

// ReadLayerFile reads a file from the layer directory or downloads from a URL.
// Returns the file content as a string.
func ReadLayerFile(path, layerDir string, ctx *BuildContext) (string, error) {
	if config.IsURL(path) {
		cachedPath, err := config.FetchFile(path, ctx.DownloadCacheDir)
		if err != nil {
			return "", fmt.Errorf("fetching %s: %w", path, err)
		}
		data, err := os.ReadFile(cachedPath)
		if err != nil {
			return "", fmt.Errorf("reading cached %s: %w", path, err)
		}
		return string(data), nil
	}

	data, err := os.ReadFile(filepath.Join(layerDir, path))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), nil
}

// copyPartitions returns a deep copy of a partition slice.
func copyPartitions(parts []PartitionDef) []PartitionDef {
	cp := make([]PartitionDef, len(parts))
	copy(cp, parts)
	return cp
}

// isValidPartitionType checks if a partition type name is recognized.
func isValidPartitionType(t string) bool {
	switch t {
	case "linux", "efi", "swap", "home", "bios-boot",
		"raid", "lvm", "microsoft-basic", "microsoft-reserved":
		return true
	}
	return false
}
