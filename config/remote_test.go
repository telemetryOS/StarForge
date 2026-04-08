package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// --- downloadToFile ---

func TestDownloadToFile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "layer content")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "layer.yaml")
	if err := downloadToFile(srv.URL+"/layer.yaml", dest); err != nil {
		t.Fatalf("downloadToFile error: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "layer content" {
		t.Errorf("content = %q, want %q", string(data), "layer content")
	}
}

func TestDownloadToFile_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.yaml")
	err := downloadToFile(srv.URL+"/missing", dest)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestDownloadToFile_NetworkError(t *testing.T) {
	// Use a non-routable address to get a connection error
	err := downloadToFile("http://192.0.2.0:9999/layer.yaml", t.TempDir()+"/out")
	if err == nil {
		t.Fatal("expected network error")
	}
}

// --- isPathTraversal ---

func TestIsPathTraversal(t *testing.T) {
	unsafe := []string{
		"..",
		"../etc/passwd",
		"../../secret",
		"../foo/bar",
		"/etc/passwd",
		"/absolute/path",
	}
	for _, p := range unsafe {
		if !isPathTraversal(p) {
			t.Errorf("isPathTraversal(%q) = false, want true", p)
		}
	}

	safe := []string{
		"scripts/setup.sh",
		"files/etc/hostname",
		"layer.yaml",
		"deep/nested/path/file.txt",
		".",
	}
	for _, p := range safe {
		if isPathTraversal(p) {
			t.Errorf("isPathTraversal(%q) = true, want false", p)
		}
	}
}
