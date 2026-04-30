package installations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTestHooksRoot replaces the package-level hooksRoot pointer for the
// duration of t and restores it via t.Cleanup.
func withTestHooksRoot(t *testing.T, root string) {
	t.Helper()
	prev := hooksRoot
	hooksRoot = root
	t.Cleanup(func() { hooksRoot = prev })
}

func TestRunInstallHooks_MissingPhaseDir_NoOp(t *testing.T) {
	root := t.TempDir()
	withTestHooksRoot(t, root)

	inst := &Installation{}
	if err := runInstallHooks("post-install", "/target", "/payload", inst); err != nil {
		t.Errorf("missing phase dir should be a clean no-op, got: %v", err)
	}
}

func TestRunInstallHooks_EmptyPhaseDir_NoOp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "post-install.d"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withTestHooksRoot(t, root)

	inst := &Installation{}
	if err := runInstallHooks("post-install", "/target", "/payload", inst); err != nil {
		t.Errorf("empty phase dir should be a clean no-op, got: %v", err)
	}
}

func TestRunInstallHooks_RunsExecutableWithArgs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "post-install.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Script writes its args to a sentinel file we can verify.
	sentinel := filepath.Join(t.TempDir(), "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$1\" \"$2\" > " + sentinel + "\necho hook-ran\n"
	if err := os.WriteFile(filepath.Join(dir, "10-test.sh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	withTestHooksRoot(t, root)

	inst := &Installation{}
	if err := runInstallHooks("post-install", "/target/rootfs", "/payload/dir", inst); err != nil {
		t.Fatalf("hook returned error: %v", err)
	}

	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("reading sentinel: %v", err)
	}
	want := "/target/rootfs\n/payload/dir\n"
	if string(got) != want {
		t.Errorf("script args = %q, want %q", string(got), want)
	}

	// Output streamed into log.
	logged := strings.Join(inst.log, "\n")
	if !strings.Contains(logged, "hook-ran") {
		t.Errorf("expected script stdout in log, got:\n%s", logged)
	}
}

func TestRunInstallHooks_LexicalOrder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "post-install.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sentinel := filepath.Join(t.TempDir(), "order.txt")

	// Create scripts out of order — runner must sort them by filename.
	for _, fn := range []string{"30-third.sh", "10-first.sh", "20-second.sh"} {
		body := "#!/bin/sh\necho " + fn + " >> " + sentinel + "\n"
		if err := os.WriteFile(filepath.Join(dir, fn), []byte(body), 0o755); err != nil {
			t.Fatalf("write %s: %v", fn, err)
		}
	}
	withTestHooksRoot(t, root)

	inst := &Installation{}
	if err := runInstallHooks("post-install", "", "", inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("reading sentinel: %v", err)
	}
	want := "10-first.sh\n20-second.sh\n30-third.sh\n"
	if string(got) != want {
		t.Errorf("execution order = %q, want %q", string(got), want)
	}
}

func TestRunInstallHooks_NonExecutableIgnored(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "post-install.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Non-executable: should be skipped silently.
	if err := os.WriteFile(filepath.Join(dir, "00-not-exec.sh"), []byte("#!/bin/sh\nexit 99\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	withTestHooksRoot(t, root)

	inst := &Installation{}
	if err := runInstallHooks("post-install", "", "", inst); err != nil {
		t.Errorf("non-executable file should be ignored, got: %v", err)
	}
}

func TestRunInstallHooks_NonZeroExitShortCircuits(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "post-install.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sentinel := filepath.Join(t.TempDir(), "ran.txt")
	scripts := map[string]string{
		"10-ok.sh":   "#!/bin/sh\necho 10-ok >> " + sentinel + "\n",
		"20-fail.sh": "#!/bin/sh\necho 20-fail >> " + sentinel + "\nexit 7\n",
		"30-skip.sh": "#!/bin/sh\necho 30-skip >> " + sentinel + "\n",
	}
	for fn, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, fn), []byte(body), 0o755); err != nil {
			t.Fatalf("write %s: %v", fn, err)
		}
	}
	withTestHooksRoot(t, root)

	inst := &Installation{}
	err := runInstallHooks("post-install", "", "", inst)
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if !strings.Contains(err.Error(), "20-fail.sh") {
		t.Errorf("error should name the failing script, got: %v", err)
	}

	// Verify execution order: 10 ran, 20 ran (and failed), 30 was skipped.
	got, _ := os.ReadFile(sentinel)
	if string(got) != "10-ok\n20-fail\n" {
		t.Errorf("ran %q, want 10-ok then 20-fail (no 30-skip)", string(got))
	}
}

func TestRunInstallHooks_DirectoryEntriesIgnored(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "post-install.d")
	if err := os.MkdirAll(filepath.Join(dir, "00-subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withTestHooksRoot(t, root)

	inst := &Installation{}
	if err := runInstallHooks("post-install", "", "", inst); err != nil {
		t.Errorf("subdirectories should be skipped, got: %v", err)
	}
}
