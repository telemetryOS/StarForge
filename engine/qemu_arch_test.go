package engine

import (
	"strings"
	"testing"
)

func TestRunQEMU_RejectsNonX86Targets(t *testing.T) {
	// The arch gate runs before any dependency or filesystem work, so this
	// call is safe without a build directory.
	err := RunQEMU("arm", "/nonexistent", "", nil, false, "", "", nil, "aarch64")
	if err == nil {
		t.Fatal("expected explicit error for aarch64 QEMU run")
	}
	if !strings.Contains(err.Error(), "not supported for arch aarch64") {
		t.Fatalf("unexpected error: %v", err)
	}
}
