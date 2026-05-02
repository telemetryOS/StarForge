package corona

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestPackExtSkipsUnallocatedBlocks(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "ext.img")
	artifact := filepath.Join(dir, "ext.corona")
	target := filepath.Join(dir, "target.blk")

	src, allocated := extFixtureImage()
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	dirty := bytes.Repeat([]byte{0xdd}, len(src))
	if err := os.WriteFile(target, dirty, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Pack(context.Background(), PackOptions{
		ImagePath:    image,
		ArtifactPath: artifact,
		ChunkSize:    4096,
		Workers:      2,
	}); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	info, err := Inspect(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.FSType != fsExt || info.FSBlockSize != 1024 {
		t.Fatalf("filesystem metadata = type %d block %d, want ext block 1024", info.FSType, info.FSBlockSize)
	}
	if info.AllocatedSHA256 != allocatedSHA(src, allocated, 1024) {
		t.Fatalf("AllocatedSHA256 = %q, want allocated-only hash", info.AllocatedSHA256)
	}
	if err := Write(context.Background(), WriteOptions{ArtifactPath: artifact, TargetPath: target}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for block, isAllocated := range allocated {
		start := block * 1024
		end := start + 1024
		if isAllocated {
			if !bytes.Equal(got[start:end], src[start:end]) {
				t.Fatalf("allocated block %d was not restored", block)
			}
			continue
		}
		if !bytes.Equal(got[start:end], dirty[start:end]) {
			t.Fatalf("unallocated block %d was overwritten", block)
		}
	}
}

func extFixtureImage() ([]byte, []bool) {
	const blockSize = 1024
	const blocks = 16
	data := make([]byte, blockSize*blocks)
	allocated := make([]bool, blocks)
	for _, block := range []int{0, 1, 2, 3, 10} {
		allocated[block] = true
	}
	for block, isAllocated := range allocated {
		if !isAllocated {
			continue
		}
		start := block * blockSize
		for i := 0; i < blockSize; i++ {
			data[start+i] = byte((block*17 + i) % 251)
		}
	}

	super := data[1024 : 1024+1024]
	clear(super)
	binary.LittleEndian.PutUint32(super[4:8], blocks)
	binary.LittleEndian.PutUint32(super[20:24], 1)
	binary.LittleEndian.PutUint32(super[24:28], 0)
	binary.LittleEndian.PutUint32(super[32:36], blocks)
	binary.LittleEndian.PutUint16(super[56:58], 0xef53)
	binary.LittleEndian.PutUint16(super[254:256], 32)

	desc := data[2048 : 2048+32]
	clear(data[2048 : 2048+1024])
	binary.LittleEndian.PutUint32(desc[0:4], 3)

	bitmap := data[3072 : 3072+1024]
	clear(bitmap)
	for block, isAllocated := range allocated {
		if isAllocated {
			bitmap[block/8] |= 1 << (block % 8)
		}
	}
	return data, allocated
}

func allocatedSHA(data []byte, allocated []bool, blockSize int) string {
	h := sha256.New()
	for block, isAllocated := range allocated {
		if !isAllocated {
			continue
		}
		start := block * blockSize
		_, _ = h.Write(data[start : start+blockSize])
	}
	return hex.EncodeToString(h.Sum(nil))
}
