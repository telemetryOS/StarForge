package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telemetryos/starforge/platform"
)

func TestStoreLifecycle(t *testing.T) {
	store := NewStore(t.TempDir(), "device", platform.Spec{Architecture: platform.ArchitectureAArch64})
	receipt, err := store.Begin("source-1")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != StateCreating {
		t.Fatalf("state = %q, want creating", receipt.State)
	}
	if err := os.WriteFile(store.CreatingDiskPath(), []byte("disk-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err = store.Seal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != StateSealed || receipt.Size != int64(len("disk-content")) || receipt.SHA256 == "" {
		t.Fatalf("unexpected sealed receipt: %#v", receipt)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceHash != "source-1" || loaded.Platform.Architecture != platform.ArchitectureAArch64 {
		t.Fatalf("unexpected loaded receipt: %#v", loaded)
	}
	if err := store.Verify(loaded); err != nil {
		t.Fatal(err)
	}
}

func TestStoreIsScopedByPlatform(t *testing.T) {
	root := t.TempDir()
	x86 := NewStore(root, "device", platform.Spec{})
	arm := NewStore(root, "device", platform.Spec{Architecture: platform.ArchitectureAArch64})
	if x86.Root() == arm.Root() {
		t.Fatal("x86-64 and AArch64 artifact stores share a path")
	}
	if filepath.Dir(x86.Root()) == filepath.Dir(arm.Root()) {
		t.Fatal("architecture is not represented in artifact path")
	}
}

func TestStoreVerifyDetectsMutation(t *testing.T) {
	store := NewStore(t.TempDir(), "device", platform.Spec{})
	receipt, err := store.Begin("source")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.CreatingDiskPath(), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err = store.Seal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.DiskPath(), []byte("mutated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(receipt); err == nil {
		t.Fatal("mutated artifact unexpectedly verified")
	}
}
