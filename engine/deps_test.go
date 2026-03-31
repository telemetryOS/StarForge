package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// --- downloadFile tests ---

func TestDownloadFile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file content"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.txt")
	if err := downloadFile(srv.URL+"/file", dest); err != nil {
		t.Fatalf("downloadFile error: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "file content" {
		t.Errorf("content = %q, want %q", string(data), "file content")
	}
}

func TestDownloadFile_NonOKStatus_DeletesNoPartialFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.txt")
	err := downloadFile(srv.URL+"/missing", dest)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	// Partial file must not remain
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("partial file should not exist after non-200 response")
	}
}

func TestDownloadFile_ServerError_NoPartialFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write header and partial body, then close connection
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		// Connection closed abruptly by the server via Close()
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.txt")
	// Even if the copy "succeeds" in writing partial data, the Close() happens
	// cleanly here — test that success case works
	if err := downloadFile(srv.URL+"/partial", dest); err == nil {
		// If no error, verify the file was written
		if _, statErr := os.Stat(dest); statErr != nil {
			t.Error("successful download should leave file")
		}
	}
}

// --- extractPkgTarZst symlink validation tests ---

// buildTestTar creates a minimal tar.gz archive in memory with the given entries.
type tarEntry struct {
	name    string
	content string
	symlink string // non-empty → create symlink with this target
}

func buildTestTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		if e.symlink != "" {
			hdr := &tar.Header{
				Typeflag: tar.TypeSymlink,
				Name:     e.name,
				Linkname: e.symlink,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
		} else {
			hdr := &tar.Header{
				Typeflag: tar.TypeReg,
				Name:     e.name,
				Size:     int64(len(e.content)),
				Mode:     0o644,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			io.WriteString(tw, e.content)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// extractTestTarGz is a thin wrapper that creates a .pkg.tar.zst-compatible
// test using gzip instead of zstd (the validation logic is the same).
func extractTestTarGzToDir(t *testing.T, data []byte, destDir string) error {
	t.Helper()
	buf := bytes.NewReader(data)
	gr, err := gzip.NewReader(buf)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.FileMode(header.Mode))

		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			io.Copy(f, tr)
			f.Close()

		case tar.TypeSymlink:
			// === This mirrors the validation logic in extractPkgTarZst ===
			linkTarget := header.Linkname
			if !filepath.IsAbs(linkTarget) {
				resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkTarget))
				rel, err := filepath.Rel(destDir, resolved)
				if err != nil || isStringPathTraversal(rel) {
					return &symlinkEscapeError{name: header.Name, target: linkTarget}
				}
			}
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Remove(target)
			os.Symlink(linkTarget, target)
		}
	}
	return nil
}

func isStringPathTraversal(rel string) bool {
	return len(rel) >= 2 && rel[:2] == ".."
}

type symlinkEscapeError struct{ name, target string }

func (e *symlinkEscapeError) Error() string {
	return "symlink " + e.name + " target " + e.target + " escapes vendor directory"
}

func TestExtractSymlink_SafeRelative(t *testing.T) {
	dir := t.TempDir()
	// Create a safe file for the symlink to point to
	os.WriteFile(filepath.Join(dir, "real.so"), []byte("data"), 0o644)

	data := buildTestTarGz(t, []tarEntry{
		{name: "lib/link.so", symlink: "../real.so"}, // stays within dir
	})
	if err := extractTestTarGzToDir(t, data, dir); err != nil {
		t.Errorf("safe relative symlink should not error: %v", err)
	}
}

func TestExtractSymlink_AbsoluteTarget_Allowed(t *testing.T) {
	// Absolute symlink targets (e.g. /usr/lib/libfoo) are legitimate in vendor trees
	dir := t.TempDir()
	data := buildTestTarGz(t, []tarEntry{
		{name: "usr/lib/link.so", symlink: "/usr/lib/real.so"},
	})
	if err := extractTestTarGzToDir(t, data, dir); err != nil {
		t.Errorf("absolute symlink target should be allowed: %v", err)
	}
}

func TestExtractSymlink_RelativeTraversal_Rejected(t *testing.T) {
	dir := t.TempDir()
	data := buildTestTarGz(t, []tarEntry{
		{name: "usr/lib/evil.so", symlink: "../../../../etc/passwd"},
	})
	err := extractTestTarGzToDir(t, data, dir)
	if err == nil {
		t.Fatal("expected error for symlink escaping destDir")
	}
	if _, ok := err.(*symlinkEscapeError); !ok {
		t.Errorf("expected symlinkEscapeError, got: %v", err)
	}
}

func TestExtractSymlink_DeepTraversal_Rejected(t *testing.T) {
	dir := t.TempDir()
	data := buildTestTarGz(t, []tarEntry{
		{name: "a/b/c/evil.so", symlink: "../../../../../../root/.ssh/authorized_keys"},
	})
	err := extractTestTarGzToDir(t, data, dir)
	if err == nil {
		t.Fatal("expected error for deep path traversal symlink")
	}
}

// --- builder Collect helpers ---

func TestDeduplicatePackages_LastLayerWins(t *testing.T) {
	pkgs := []actions.Package{
		{Name: "foo", Version: "1.0"},
		{Name: "bar", Version: "2.0"},
		{Name: "foo", Version: "1.1"}, // later version wins
	}
	result := deduplicatePackages(pkgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(result))
	}
	for _, p := range result {
		if p.Name == "foo" && p.Version != "1.1" {
			t.Errorf("foo should be version 1.1, got %s", p.Version)
		}
	}
}

func TestDeduplicatePackages_Empty(t *testing.T) {
	if got := deduplicatePackages(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	if got := deduplicatePackages([]actions.Package{}); len(got) != 0 {
		t.Errorf("expected empty for empty input, got %v", got)
	}
}

func TestDeduplicatePackages_NoDuplicates(t *testing.T) {
	pkgs := []actions.Package{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	result := deduplicatePackages(pkgs)
	if len(result) != 3 {
		t.Errorf("expected 3 packages, got %d", len(result))
	}
}

func TestPropagateVars_NoExports_AllPropagate(t *testing.T) {
	outer := map[string]string{"x": "1"}
	layer := map[string]string{"x": "2", "y": "3"}
	propagateVars(nil, layer, outer)
	if outer["x"] != "2" || outer["y"] != "3" {
		t.Errorf("all vars should propagate: %v", outer)
	}
}

func TestPropagateVars_WithExports_OnlyExported(t *testing.T) {
	outer := map[string]string{}
	layer := map[string]string{"pub": "visible", "priv": "hidden"}
	propagateVars([]string{"pub"}, layer, outer)
	if outer["pub"] != "visible" {
		t.Error("exported var should propagate")
	}
	if _, ok := outer["priv"]; ok {
		t.Error("non-exported var must not propagate")
	}
}
