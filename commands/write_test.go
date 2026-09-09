package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerTempDirectoryUsesStarForgeTempDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("STARFORGE_TMPDIR", base)

	dir, err := createInstallerTempDir()
	if err != nil {
		t.Fatalf("create installer temp directory: %v", err)
	}
	defer os.RemoveAll(dir)

	if got := filepath.Dir(dir); got != base {
		t.Fatalf("installer temp directory parent = %q, want %q", got, base)
	}
}
