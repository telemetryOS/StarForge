package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitOutput_CreatesLogFile(t *testing.T) {
	dir := t.TempDir()

	o, err := InitOutput(dir, "test", "target")
	if err != nil {
		t.Fatalf("InitOutput: %v", err)
	}
	defer o.Close()

	logPath := filepath.Join(dir, "build.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("build.log not created: %v", err)
	}
}

func TestInitOutput_LogContainsTimestamp(t *testing.T) {
	dir := t.TempDir()

	o, err := InitOutput(dir, "test", "target")
	if err != nil {
		t.Fatalf("InitOutput: %v", err)
	}
	o.Close()

	data, err := os.ReadFile(filepath.Join(dir, "build.log"))
	if err != nil {
		t.Fatalf("reading build.log: %v", err)
	}
	log := string(data)

	if !strings.Contains(log, "Build started: ") {
		t.Errorf("build.log missing start timestamp:\n%s", log)
	}
	if !strings.Contains(log, "Build ended: ") {
		t.Errorf("build.log missing end timestamp:\n%s", log)
	}
}

func TestInitOutput_LogCapturesOutput(t *testing.T) {
	dir := t.TempDir()

	o, err := InitOutput(dir, "test", "target")
	if err != nil {
		t.Fatalf("InitOutput: %v", err)
	}

	o.Header("Test Header")
	o.Info("test info %s", "value")
	o.SubInfo("sub info")
	o.Close()

	data, err := os.ReadFile(filepath.Join(dir, "build.log"))
	if err != nil {
		t.Fatalf("reading build.log: %v", err)
	}
	log := string(data)

	if !strings.Contains(log, "Test Header") {
		t.Errorf("build.log missing Header output:\n%s", log)
	}
	if !strings.Contains(log, "test info value") {
		t.Errorf("build.log missing Info output:\n%s", log)
	}
	if !strings.Contains(log, "sub info") {
		t.Errorf("build.log missing SubInfo output:\n%s", log)
	}
}

func TestInitOutput_Reentrant(t *testing.T) {
	dir := t.TempDir()

	o1, err := InitOutput(dir, "test", "target")
	if err != nil {
		t.Fatalf("InitOutput: %v", err)
	}

	// Second call should return the same instance (nested builds)
	o2, err := InitOutput(dir, "test", "target")
	if err != nil {
		t.Fatalf("InitOutput (second): %v", err)
	}

	if o1 != o2 {
		t.Error("expected same Output instance for nested calls")
	}

	o1.Close()
}

func TestInitOutput_NoBuildDir(t *testing.T) {
	// Pass empty string — no log file
	o, err := InitOutput("", "test", "target")
	if err != nil {
		t.Fatalf("InitOutput: %v", err)
	}
	defer o.Close()

	// Should work without crashing
	o.Header("no log")
	o.Info("test")
}

func TestOutput_RunWithSpinner_NonInteractive(t *testing.T) {
	dir := t.TempDir()

	o, err := InitOutput(dir, "test", "target")
	if err != nil {
		t.Fatalf("InitOutput: %v", err)
	}
	defer o.Close()

	// In test mode, output is non-interactive (no terminal)
	called := false
	err = o.RunWithSpinner("test label", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithSpinner: %v", err)
	}
	if !called {
		t.Error("RunWithSpinner did not call fn")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"milliseconds", 50 * time.Millisecond, "50ms"},
		{"seconds", 2500 * time.Millisecond, "2.5s"},
		{"minutes", 125 * time.Second, "2m5s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}
