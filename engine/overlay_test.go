package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlayManager_Init_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	om := NewOverlayManager(dir)

	if err := om.Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	for _, sub := range []string{"cache", "merged", "work"} {
		p := filepath.Join(dir, sub)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected directory %s to exist: %v", p, err)
		}
	}
}

func TestOverlayManager_Unmount_WhenNotMounted_IsNoop(t *testing.T) {
	dir := t.TempDir()
	om := NewOverlayManager(dir)
	// mounted is false by default — Unmount must be a no-op (no exec calls)
	if err := om.Unmount(); err != nil {
		t.Fatalf("Unmount() on unmounted overlay returned error: %v", err)
	}
}

func TestOverlayManager_PhaseUpperDir(t *testing.T) {
	om := NewOverlayManager("/build")
	for i, name := range PhaseNames {
		got := om.PhaseUpperDir(i)
		want := filepath.Join("/build", "cache", name, "upper")
		if got != want {
			t.Errorf("PhaseUpperDir(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestOverlayManager_MountedFlagOnlySetAfterSuccessfulUmount(t *testing.T) {
	// If mounted=false before unmounting and the umount call succeeds, the flag
	// should remain false after the call. Verify via the Unmount() public method
	// — when already unmounted it's a no-op.
	dir := t.TempDir()
	om := NewOverlayManager(dir)

	// The field is false, Unmount must return nil without touching the filesystem
	for i := 0; i < 3; i++ {
		if err := om.Unmount(); err != nil {
			t.Fatalf("call %d: Unmount() on unmounted overlay returned error: %v", i, err)
		}
		if om.mounted {
			t.Fatalf("call %d: mounted flag became true unexpectedly", i)
		}
	}
}
