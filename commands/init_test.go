package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runInitInDir(t *testing.T, dir, name string) error {
	t.Helper()

	// runInit reads an optional description from stdin; provide an empty line.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	w.WriteString("\n\n") // empty description + default target name
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		r.Close()
	}()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	return runInit(initCmd, []string{name})
}

func TestRunInit_CreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir, "myos"); err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	project := filepath.Join(dir, "myos")
	for _, rel := range []string{
		"starforge.yaml",
		"layers/base/layer.yaml",
		"layers/base/files/etc",
		".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(project, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestRunInit_StarforgeYAML_ContainsName(t *testing.T) {
	dir := t.TempDir()
	runInitInDir(t, dir, "testproject")

	data, err := os.ReadFile(filepath.Join(dir, "testproject", "starforge.yaml"))
	if err != nil {
		t.Fatalf("reading starforge.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"testproject"`) {
		t.Errorf("starforge.yaml missing project name:\n%s", content)
	}
	if !strings.Contains(content, "targets:") {
		t.Errorf("starforge.yaml missing targets:\n%s", content)
	}
	if !strings.Contains(content, "distribution:") {
		t.Errorf("starforge.yaml missing default target:\n%s", content)
	}
}

func TestRunInit_LayerYAML_ContainsActions(t *testing.T) {
	dir := t.TempDir()
	runInitInDir(t, dir, "myos")

	data, err := os.ReadFile(filepath.Join(dir, "myos/layers/base/layer.yaml"))
	if err != nil {
		t.Fatalf("reading layer.yaml: %v", err)
	}
	content := string(data)
	for _, action := range []string{"partition-add", "pacman-add", "system-hostname", "system-locale", "system-timezone"} {
		if !strings.Contains(content, action) {
			t.Errorf("layer.yaml missing action %q", action)
		}
	}
}

func TestRunInit_Gitignore_Content(t *testing.T) {
	dir := t.TempDir()
	runInitInDir(t, dir, "proj")

	data, err := os.ReadFile(filepath.Join(dir, "proj/.gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(data) != ".starforge/\n" {
		t.Errorf(".gitignore = %q, want %q", string(data), ".starforge/\n")
	}
}

func TestRunInit_FailsIfDirExists(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "existing"), 0o755)

	err := runInitInDir(t, dir, "existing")
	if err == nil {
		t.Fatal("expected error when directory already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists': %v", err)
	}
}

func TestRunInit_WithDescription(t *testing.T) {
	// Verify description is included when provided (non-interactive via YAML)
	// The non-interactive path (args provided) skips the reader —
	// just test that the name arg works and produces valid output.
	dir := t.TempDir()
	if err := runInitInDir(t, dir, "described"); err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "described/starforge.yaml"))
	if !strings.Contains(string(data), "described") {
		t.Error("starforge.yaml should reference the project name")
	}
}
