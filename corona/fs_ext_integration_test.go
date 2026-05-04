package corona

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtFilesystemRoundTripThroughCorona(t *testing.T) {
	requireExtTools(t)
	dir := t.TempDir()
	image := filepath.Join(dir, "root.img")
	corona := filepath.Join(dir, "root.corona")
	target := filepath.Join(dir, "target.img")

	createExtImage(t, image)
	assertExtFilesystem(t, image)
	dirtyTargetFromImage(t, image, target)
	if err := create(context.Background(), createOptions{
		SourcePath: image,
		CoronaPath: corona,
		ChunkSize:  4096,
		Workers:    4,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeCoronaToRegularForTest(t, corona, target, 4, WriteOrderStriped); err != nil {
		t.Fatal(err)
	}

	assertExtFilesystem(t, target)
}

func TestExtFilesystemRoundTripThroughDirectImageWrite(t *testing.T) {
	requireExtTools(t)
	dir := t.TempDir()
	image := filepath.Join(dir, "root.img")
	target := filepath.Join(dir, "target.img")

	createExtImage(t, image)
	assertExtFilesystem(t, image)
	dirtyTargetFromImage(t, image, target)
	if err := writeImageToRegularForTest(t, image, target, 4096, 4, WriteOrderStriped); err != nil {
		t.Fatal(err)
	}

	assertExtFilesystem(t, target)
}

func requireExtTools(t *testing.T) {
	t.Helper()
	for _, name := range []string{"mkfs.ext4", "e2fsck", "debugfs"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is required for ext integration test", name)
		}
	}
}

func createExtImage(t *testing.T, image string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "rootfs")
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "var", "lib", "telemetry"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc", "telemetry.conf"), []byte("channel=stable\nserial=test-unit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("0123456789abcdef"), 4096)
	if err := os.WriteFile(filepath.Join(root, "var", "lib", "telemetry", "payload.bin"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(image)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("mkfs.ext4", "-q", "-F", "-d", root, image)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mkfs.ext4: %v\n%s", err, out)
	}
}

func dirtyTargetFromImage(t *testing.T, image, target string) {
	t.Helper()
	info, err := os.Stat(image)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, bytes.Repeat([]byte{0xa5}, int(info.Size())), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertExtFilesystem(t *testing.T, image string) {
	t.Helper()
	if out, err := exec.Command("e2fsck", "-f", "-n", image).CombinedOutput(); err != nil {
		t.Fatalf("e2fsck: %v\n%s", err, out)
	}
	assertDebugfsCat(t, image, "/etc/telemetry.conf", []byte("channel=stable\nserial=test-unit\n"))
	assertDebugfsCat(t, image, "/var/lib/telemetry/payload.bin", bytes.Repeat([]byte("0123456789abcdef"), 4096))
}

func assertDebugfsCat(t *testing.T, image, path string, want []byte) {
	t.Helper()
	out, err := exec.Command("debugfs", "-R", "cat "+path, image).Output()
	if err != nil {
		t.Fatalf("debugfs cat %s: %v", path, err)
	}
	if !bytes.Equal(out, want) {
		for i := 0; i < len(out) && i < len(want); i++ {
			if out[i] != want[i] {
				t.Fatalf("debugfs cat %s byte %d = 0x%02x, want 0x%02x", path, i, out[i], want[i])
			}
		}
		t.Fatalf("debugfs cat %s returned %d bytes, want %d", path, len(out), len(want))
	}
}
