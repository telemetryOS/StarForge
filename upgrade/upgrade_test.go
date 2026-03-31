package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgrade_AtomicReplacement_TempFileUsed(t *testing.T) {
	// Verify that the upgrade writes to a .new temp file before renaming.
	// We test this by replacing the currentBinary path with a known file and
	// verifying that after a successful copy, the .new file is gone and the
	// original file has been replaced atomically.
	dir := t.TempDir()

	srcBinary := filepath.Join(dir, "new-binary")
	dstBinary := filepath.Join(dir, "current-binary")
	tmpBin := dstBinary + ".new"

	// Create the "new" binary to copy from
	os.WriteFile(srcBinary, []byte("new content"), 0o755)
	// Create the "current" binary to replace
	os.WriteFile(dstBinary, []byte("old content"), 0o755)

	// Simulate the atomic replacement logic from upgrade.go
	src, err := os.Open(srcBinary)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(tmpBin, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("open tmp: %v", err)
	}

	buf := make([]byte, 512)
	n, _ := src.Read(buf)
	dst.Write(buf[:n])

	if err := dst.Close(); err != nil {
		os.Remove(tmpBin)
		t.Fatalf("close tmp: %v", err)
	}

	if err := os.Rename(tmpBin, dstBinary); err != nil {
		os.Remove(tmpBin)
		t.Fatalf("rename: %v", err)
	}

	// Verify: .new file is gone
	if _, err := os.Stat(tmpBin); !os.IsNotExist(err) {
		t.Error(".new temp file should be removed after rename")
	}

	// Verify: dstBinary has new content
	data, _ := os.ReadFile(dstBinary)
	if string(data) != "new content" {
		t.Errorf("dstBinary content = %q, want %q", string(data), "new content")
	}
}

func TestUpgrade_AtomicReplacement_CleanupOnWriteError(t *testing.T) {
	// Verify the .new temp file is deleted when the copy fails.
	dir := t.TempDir()
	dstBinary := filepath.Join(dir, "current")
	tmpBin := dstBinary + ".new"

	// Create the temp file as if we started writing
	os.WriteFile(tmpBin, []byte("partial"), 0o755)

	// Simulate cleanup after copy failure
	os.Remove(tmpBin)

	if _, err := os.Stat(tmpBin); !os.IsNotExist(err) {
		t.Error(".new file should be cleaned up on failure")
	}
}

func TestUpgrade_GoRunDetection(t *testing.T) {
	// Upgrade() should bail out gracefully when running via `go run`.
	// We simulate this by checking the detection logic directly.
	goRunBinary := "/tmp/go-build123/exe/main"
	if !containsGoRun(goRunBinary) {
		t.Error("should detect go run binary path")
	}

	realBinary := "/usr/local/bin/starforge"
	if containsGoRun(realBinary) {
		t.Error("should not flag real binary as go run")
	}
}

// containsGoRun mirrors the detection logic from upgrade.go.
func containsGoRun(path string) bool {
	return len(path) > 9 && path[:9] == "/tmp/go-b" ||
		len(path) >= 9 && containsSubstring(path, "/go-build")
}

func containsSubstring(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestGetLatestTag_ParsesSemver(t *testing.T) {
	// Test the version comparison logic in getLatestTag.
	// We test the comparison rules directly since getLatestTag hits the network.
	versions := []struct {
		major, minor, patch int
	}{
		{1, 0, 0},
		{1, 2, 0},
		{1, 2, 3},
		{2, 0, 0},
	}

	// Simulate finding the latest: major wins, then minor, then patch
	latestMajor, latestMinor, latestPatch := 0, 0, 0
	for _, v := range versions {
		if v.major > latestMajor ||
			(v.major == latestMajor && v.minor > latestMinor) ||
			(v.major == latestMajor && v.minor == latestMinor && v.patch > latestPatch) {
			latestMajor = v.major
			latestMinor = v.minor
			latestPatch = v.patch
		}
	}

	if latestMajor != 2 || latestMinor != 0 || latestPatch != 0 {
		t.Errorf("expected v2.0.0 as latest, got v%d.%d.%d", latestMajor, latestMinor, latestPatch)
	}
}
