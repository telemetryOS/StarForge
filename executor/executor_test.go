package executor

import "testing"

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
