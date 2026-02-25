package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartBuildLog_CapturesStdout(t *testing.T) {
	dir := t.TempDir()

	cleanup, err := startBuildLog(dir)
	if err != nil {
		t.Fatalf("startBuildLog: %v", err)
	}

	fmt.Println("hello from stdout")
	fmt.Fprintln(os.Stderr, "hello from stderr")

	cleanup()

	data, err := os.ReadFile(filepath.Join(dir, "build.log"))
	if err != nil {
		t.Fatalf("reading build.log: %v", err)
	}
	log := string(data)

	if !strings.Contains(log, "hello from stdout") {
		t.Errorf("build.log missing stdout output:\n%s", log)
	}
	if !strings.Contains(log, "hello from stderr") {
		t.Errorf("build.log missing stderr output:\n%s", log)
	}
}

func TestStartBuildLog_CapturesSubprocess(t *testing.T) {
	dir := t.TempDir()

	cleanup, err := startBuildLog(dir)
	if err != nil {
		t.Fatalf("startBuildLog: %v", err)
	}

	// Subprocess inherits os.Stdout/os.Stderr which are now the pipe write-ends
	cmd := exec.Command("echo", "subprocess output")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("subprocess: %v", err)
	}

	cleanup()

	data, err := os.ReadFile(filepath.Join(dir, "build.log"))
	if err != nil {
		t.Fatalf("reading build.log: %v", err)
	}
	if !strings.Contains(string(data), "subprocess output") {
		t.Errorf("build.log missing subprocess output:\n%s", string(data))
	}
}

func TestStartBuildLog_HasTimestampHeader(t *testing.T) {
	dir := t.TempDir()

	cleanup, err := startBuildLog(dir)
	if err != nil {
		t.Fatalf("startBuildLog: %v", err)
	}
	cleanup()

	data, err := os.ReadFile(filepath.Join(dir, "build.log"))
	if err != nil {
		t.Fatalf("reading build.log: %v", err)
	}
	log := string(data)

	if !strings.HasPrefix(log, "Build started: ") {
		t.Errorf("build.log should start with timestamp header, got:\n%s", log)
	}
	if !strings.Contains(log, "Build ended: ") {
		t.Errorf("build.log should contain end timestamp, got:\n%s", log)
	}
}

func TestStartBuildLog_RestoresGlobals(t *testing.T) {
	dir := t.TempDir()

	origOut := os.Stdout
	origErr := os.Stderr

	cleanup, err := startBuildLog(dir)
	if err != nil {
		t.Fatalf("startBuildLog: %v", err)
	}

	// While logging, globals should be different
	if os.Stdout == origOut {
		t.Error("os.Stdout should be replaced during logging")
	}
	if os.Stderr == origErr {
		t.Error("os.Stderr should be replaced during logging")
	}

	cleanup()

	// After cleanup, globals should be restored
	if os.Stdout != origOut {
		t.Error("os.Stdout not restored after cleanup")
	}
	if os.Stderr != origErr {
		t.Error("os.Stderr not restored after cleanup")
	}
}

func TestWrapBuildLog_NonFatal(t *testing.T) {
	// Point to a non-existent directory — should warn but not panic
	cleanup := wrapBuildLog("/nonexistent/path/that/does/not/exist")
	cleanup() // should be a no-op, not panic
}
