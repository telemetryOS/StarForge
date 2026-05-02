package corona

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteImageLargeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "direct.img")
	target := filepath.Join(dir, "direct.target")

	src := make([]byte, 10*1024*1024+321)
	for i := range src {
		if i > 1024*1024 && i < 4*1024*1024 {
			src[i] = byte((i * 17) % 251)
		}
		if i > 8*1024*1024 {
			src[i] = byte((i / 31) % 239)
		}
	}
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, bytes.Repeat([]byte{0x77}, len(src)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteImage(context.Background(), WriteImageOptions{
		ImagePath:  image,
		TargetPath: target,
		ChunkSize:  512 * 1024,
		Workers:    4,
		WriteOrder: WriteOrderStriped,
	}); err != nil {
		t.Fatalf("WriteImage: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("direct image write mismatch")
	}
}

func TestWriteRejectsTooSmallTargetBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "image.img")
	artifact := filepath.Join(dir, "image.corona")
	target := filepath.Join(dir, "target.blk")

	src := bytes.Repeat([]byte{0x7d}, 8192)
	before := bytes.Repeat([]byte{0xa5}, 4096)
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, before, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Pack(context.Background(), PackOptions{
		ImagePath:    image,
		ArtifactPath: artifact,
		ChunkSize:    4096,
	}); err != nil {
		t.Fatal(err)
	}

	err := Write(context.Background(), WriteOptions{ArtifactPath: artifact, TargetPath: target})
	if err == nil || !strings.Contains(err.Error(), "target too small") {
		t.Fatalf("Write err = %v, want target too small", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("target changed despite capacity preflight failure")
	}
}

func TestScheduleStriped(t *testing.T) {
	jobs := []chunkJob{
		{offset: 0},
		{offset: 1},
		{offset: 2},
		{offset: 3},
		{offset: 4},
	}
	got := scheduleChunkJobs(jobs, WriteOrderStriped, 2)
	var offsets []uint64
	for _, job := range got {
		offsets = append(offsets, job.offset)
	}
	want := []uint64{0, 2, 4, 1, 3}
	if !equalUint64s(offsets, want) {
		t.Fatalf("schedule = %v, want %v", offsets, want)
	}
}

func equalUint64s(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
