package corona

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestPackInspectWriteReconstructsDirtyTarget(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "root.img")
	artifact := filepath.Join(dir, "root.corona")
	target := filepath.Join(dir, "target.blk")

	src := make([]byte, 96*1024)
	copy(src[4096:8192], bytes.Repeat([]byte{0x11}, 4096))
	copy(src[48*1024:52*1024], bytes.Repeat([]byte{0x22}, 4096))
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, bytes.Repeat([]byte{0xff}, len(src)), 0644); err != nil {
		t.Fatal(err)
	}

	var createProgress []int
	if err := Pack(context.Background(), PackOptions{
		ImagePath:    image,
		ArtifactPath: artifact,
		ChunkSize:    4096,
		Workers:      2,
		Progress: func(p Progress) {
			createProgress = append(createProgress, p.Percent)
		},
	}); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	info, err := Inspect(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.ImageSize != uint64(len(src)) {
		t.Fatalf("ImageSize = %d, want %d", info.ImageSize, len(src))
	}
	if info.OperationNum == 0 {
		t.Fatal("expected operations")
	}
	allocatedSHA256 := sha256.Sum256(src)
	if info.AllocatedSHA256 != hex.EncodeToString(allocatedSHA256[:]) {
		t.Fatalf("AllocatedSHA256 = %q, want %q", info.AllocatedSHA256, hex.EncodeToString(allocatedSHA256[:]))
	}
	if len(createProgress) == 0 || createProgress[len(createProgress)-1] != 100 {
		t.Fatalf("create progress = %v, want final 100", createProgress)
	}

	var flashProgress []int
	if err := Write(context.Background(), WriteOptions{
		ArtifactPath: artifact,
		TargetPath:   target,
		Workers:      3,
		WriteOrder:   WriteOrderStriped,
		Progress: func(p Progress) {
			flashProgress = append(flashProgress, p.Percent)
		},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("flashed target does not match source image")
	}
	if len(flashProgress) == 0 || flashProgress[len(flashProgress)-1] != 100 {
		t.Fatalf("flash progress = %v, want final 100", flashProgress)
	}
}

func TestPackWriteLargeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "large.img")
	artifact := filepath.Join(dir, "large.corona")
	target := filepath.Join(dir, "large.target")

	src := make([]byte, 18*1024*1024+12345)
	for i := range src {
		switch {
		case i < 2*1024*1024:
			// leave a large zero run
		case i >= 5*1024*1024 && i < 9*1024*1024:
			src[i] = byte((i * 31) % 251)
		case i >= 13*1024*1024:
			src[i] = byte((i / 97) % 253)
		}
	}
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, bytes.Repeat([]byte{0xa5}, len(src)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Pack(context.Background(), PackOptions{
		ImagePath:    image,
		ArtifactPath: artifact,
		ChunkSize:    1024 * 1024,
		Workers:      4,
	}); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if err := Write(context.Background(), WriteOptions{
		ArtifactPath: artifact,
		TargetPath:   target,
		Workers:      4,
		WriteOrder:   WriteOrderStriped,
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("large round trip mismatch")
	}
}

func createFixtureArtifact(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	image := filepath.Join(dir, "fixture.img")
	artifact := filepath.Join(dir, "fixture.corona")
	data := append(bytes.Repeat([]byte{0}, 4096), bytes.Repeat([]byte("abcd"), 2048)...)
	if err := os.WriteFile(image, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Pack(context.Background(), PackOptions{
		ImagePath:    image,
		ArtifactPath: artifact,
		ChunkSize:    4096,
		Workers:      1,
	}); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return artifact
}
