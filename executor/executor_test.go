package executor

import (
	"testing"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/platform"
)

func TestParseMode(t *testing.T) {
	for _, value := range []string{"auto", "native", "vm"} {
		mode, err := ParseMode(value)
		if err != nil {
			t.Errorf("ParseMode(%q): %v", value, err)
		}
		if string(mode) != value {
			t.Errorf("ParseMode(%q) = %q", value, mode)
		}
	}
	if _, err := ParseMode("container"); err == nil {
		t.Fatal("unsupported mode unexpectedly succeeded")
	}
}

func TestNewNativeAndAuto(t *testing.T) {
	for _, mode := range []Mode{ModeAuto, ModeNative} {
		exec, err := New(mode)
		if err != nil {
			t.Fatalf("New(%q): %v", mode, err)
		}
		if exec.Mode() != ModeNative {
			t.Errorf("New(%q).Mode() = %q, want native", mode, exec.Mode())
		}
	}
}

func TestNativeRejectsCrossArchitecture(t *testing.T) {
	host, err := platform.HostArchitecture()
	if err != nil {
		t.Skip(err)
	}
	target := platform.ArchitectureAArch64
	if host == target {
		target = platform.ArchitectureX86_64
	}
	request := BuildRequest{Target: config.Target{Platform: platform.Spec{Architecture: target}}}
	if err := validateNativeArchitecture(request); err == nil {
		t.Fatal("cross-architecture native build unexpectedly passed validation")
	}
}
