package corona

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlashImageLargeRoundTrip(t *testing.T) {
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
	if err := writeImageToRegularForTest(t, image, target, 512*1024, 4, WriteOrderStriped); err != nil {
		t.Fatalf("FlashImage: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("direct image write mismatch")
	}
}

func TestFlashImageRejectsRegularFileTarget(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "image.img")
	target := filepath.Join(dir, "target.img")
	if err := os.WriteFile(image, bytes.Repeat([]byte{0x11}, 8192), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, bytes.Repeat([]byte{0x22}, 8192), 0644); err != nil {
		t.Fatal(err)
	}
	err := flashImage(context.Background(), flashImageOptions{ImagePath: image, TargetPath: target, ChunkSize: 4096})
	if err == nil || !strings.Contains(err.Error(), "block device") {
		t.Fatalf("FlashImage err = %v, want block-device rejection", err)
	}
}

func TestExtractImageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "image.img")
	corona := filepath.Join(dir, "image.corona")
	unpacked := filepath.Join(dir, "unpacked.img")

	src := make([]byte, 2*1024*1024+777)
	for i := range src {
		src[i] = byte((i * 19) % 251)
	}
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := create(context.Background(), createOptions{
		SourcePath: image,
		CoronaPath: corona,
		ChunkSize:  256 * 1024,
		Workers:    3,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := extractImage(context.Background(), extractImageOptions{
		CoronaPath: corona,
		ImagePath:  unpacked,
		Workers:    3,
		WriteOrder: WriteOrderStriped,
	}); err != nil {
		t.Fatalf("ExtractImage: %v", err)
	}
	got, err := os.ReadFile(unpacked)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("unpacked image mismatch")
	}
}

func validateCoronaForTest(t *testing.T, coronaPath string) (Info, error) {
	t.Helper()
	corona, err := os.Open(coronaPath)
	if err != nil {
		return Info{}, err
	}
	defer corona.Close()
	coronaInfo, err := corona.Stat()
	if err != nil {
		return Info{}, err
	}
	header, err := readFileHeader(corona)
	if err != nil {
		return Info{}, err
	}
	targetPath := filepath.Join(t.TempDir(), "target.img")
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return Info{}, err
	}
	defer target.Close()
	if err := target.Truncate(int64(header.imageSize)); err != nil {
		return Info{}, err
	}
	return writeCoronaFrames(context.Background(), corona, target, header, uint64(coronaInfo.Size()), 1, WriteOrderSequential, nil, true, false)
}

func TestFlashRejectsTooSmallTargetBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "image.img")
	corona := filepath.Join(dir, "image.corona")
	target := filepath.Join(dir, "target.blk")

	src := bytes.Repeat([]byte{0x7d}, 8192)
	before := bytes.Repeat([]byte{0xa5}, 4096)
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, before, 0644); err != nil {
		t.Fatal(err)
	}
	if err := create(context.Background(), createOptions{
		SourcePath: image,
		CoronaPath: corona,
		ChunkSize:  4096,
	}); err != nil {
		t.Fatal(err)
	}

	err := writeCoronaToRegularForTest(t, corona, target, 0, WriteOrderSequential)
	if err == nil || !strings.Contains(err.Error(), "target too small") {
		t.Fatalf("Flash err = %v, want target too small", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("target changed despite capacity preflight failure")
	}
}

func writeCoronaToRegularForTest(t *testing.T, coronaPath, targetPath string, workers int, order WriteOrder) error {
	t.Helper()
	return writeCoronaToRegularForTestWithProgress(t, coronaPath, targetPath, workers, order, nil)
}

func writeCoronaToRegularForTestWithProgress(t *testing.T, coronaPath, targetPath string, workers int, order WriteOrder, progress func(Progress)) error {
	t.Helper()
	corona, err := os.Open(coronaPath)
	if err != nil {
		return err
	}
	defer corona.Close()
	coronaInfo, err := corona.Stat()
	if err != nil {
		return err
	}
	header, err := readFileHeader(corona)
	if err != nil {
		return err
	}
	target, err := os.OpenFile(targetPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer target.Close()
	if err := validateTargetCapacity(target, header.imageSize); err != nil {
		return err
	}
	_, err = writeCoronaFrames(context.Background(), corona, target, header, uint64(coronaInfo.Size()), workers, order, progress, false, false)
	return err
}

func writeImageToRegularForTest(t *testing.T, imagePath, targetPath string, chunkSize int64, workers int, order WriteOrder) error {
	t.Helper()
	src, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	target, err := os.OpenFile(targetPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer target.Close()
	imageSize := uint64(info.Size())
	if err := validateTargetCapacity(target, imageSize); err != nil {
		return err
	}
	alloc := detectAllocationChecker(src, imageSize)
	return writeImageChunks(context.Background(), src, target, imageSize, uint64(chunkSize), workers, order, nil, alloc, false, false)
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
