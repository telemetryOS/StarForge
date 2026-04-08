package payloads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePayloadDir_RejectsPathTraversal(t *testing.T) {
	unsafe := []string{
		"..",
		"../etc",
		"../../root",
		"foo/bar", // contains separator
	}
	for _, name := range unsafe {
		_, err := ResolvePayloadDir("/tmp", name)
		if err == nil {
			t.Errorf("ResolvePayloadDir(%q) should reject traversal, got nil error", name)
		}
	}
}

func TestResolvePayloadDir_RejectsDot(t *testing.T) {
	_, err := ResolvePayloadDir("/tmp", ".")
	if err == nil {
		t.Fatal("ResolvePayloadDir('.') should return error")
	}
}

func TestResolvePayloadDir_FindsNested(t *testing.T) {
	base := t.TempDir()
	nestedDir := filepath.Join(base, "myapp")
	os.MkdirAll(nestedDir, 0o755)
	os.WriteFile(filepath.Join(nestedDir, "manifest.json"), []byte("{}"), 0o644)

	got, err := ResolvePayloadDir(base, "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nestedDir {
		t.Errorf("got %q, want %q", got, nestedDir)
	}
}

func TestResolvePayloadDir_FallsBackToFlat(t *testing.T) {
	base := t.TempDir()
	os.WriteFile(filepath.Join(base, "manifest.json"), []byte("{}"), 0o644)

	got, err := ResolvePayloadDir(base, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != base {
		t.Errorf("got %q, want %q (flat layout)", got, base)
	}
}

func TestResolvePayloadDir_NestedTakesPrecedence(t *testing.T) {
	base := t.TempDir()
	// Both flat and nested manifest exist
	os.WriteFile(filepath.Join(base, "manifest.json"), []byte("{}"), 0o644)
	nestedDir := filepath.Join(base, "app")
	os.MkdirAll(nestedDir, 0o755)
	os.WriteFile(filepath.Join(nestedDir, "manifest.json"), []byte("{}"), 0o644)

	got, err := ResolvePayloadDir(base, "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nestedDir {
		t.Errorf("nested should take precedence: got %q, want %q", got, nestedDir)
	}
}

func TestResolvePayloadDir_NotFound(t *testing.T) {
	base := t.TempDir()
	_, err := ResolvePayloadDir(base, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing payload")
	}
}
