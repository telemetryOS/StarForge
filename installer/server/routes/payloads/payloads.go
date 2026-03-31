package payloads

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolvePayloadDir locates the payload directory for the given name.
// It checks for a nested layout (baseDir/name/manifest.json) first, then
// falls back to a flat layout (baseDir/manifest.json).
func ResolvePayloadDir(baseDir, name string) (string, error) {
	// Reject names that could traverse outside baseDir.
	if name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid payload name %q", name)
	}

	// Nested layout: baseDir/name/manifest.json
	nested := filepath.Join(baseDir, name)
	if _, err := os.Stat(filepath.Join(nested, "manifest.json")); err == nil {
		return nested, nil
	}

	// Flat layout: baseDir/manifest.json (single payload)
	if _, err := os.Stat(filepath.Join(baseDir, "manifest.json")); err == nil {
		return baseDir, nil
	}

	return "", fmt.Errorf("payload %q not found in %s", name, baseDir)
}
