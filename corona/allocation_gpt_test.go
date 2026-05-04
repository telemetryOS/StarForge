package corona

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestGPTPlannerSkipsDiskGapsAndPartitionUnallocatedBlocks(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "disk.img")
	corona := filepath.Join(dir, "disk.corona")
	target := filepath.Join(dir, "target.blk")

	src, partStart, allocated := gptExtFixtureImage()
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	dirty := bytes.Repeat([]byte{0xee}, len(src))
	if err := os.WriteFile(target, dirty, 0644); err != nil {
		t.Fatal(err)
	}
	if err := create(context.Background(), createOptions{
		SourcePath: image,
		CoronaPath: corona,
		ChunkSize:  4096,
		Workers:    3,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeCoronaToRegularForTest(t, corona, target, 3, WriteOrderSequential); err != nil {
		t.Fatalf("Flash: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		switch {
		case i < 17408:
			if got[i] != src[i] {
				t.Fatalf("primary GPT byte %d was not restored", i)
			}
		case i >= len(src)-16896:
			if got[i] != src[i] {
				t.Fatalf("backup GPT byte %d was not restored", i)
			}
		case i >= partStart && i < partStart+len(allocated)*1024:
			block := (i - partStart) / 1024
			if allocated[block] {
				if got[i] != src[i] {
					t.Fatalf("allocated partition byte %d was not restored", i)
				}
			} else if got[i] != dirty[i] {
				t.Fatalf("unallocated partition byte %d was overwritten", i)
			}
		default:
			if got[i] != dirty[i] {
				t.Fatalf("disk gap byte %d was overwritten", i)
			}
		}
	}
}

func TestCaptureImageUsesGPTAllocationPlanner(t *testing.T) {
	dir := t.TempDir()
	device := filepath.Join(dir, "device.blk")
	image := filepath.Join(dir, "captured.img")

	src, partStart, allocated := gptExtFixtureImage()
	fillSkippableRangesWithGarbage(src, partStart, allocated)
	if err := os.WriteFile(device, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := captureImage(context.Background(), captureImageOptions{
		SourcePath: device,
		ImagePath:  image,
		ChunkSize:  4096,
		Workers:    3,
	}); err != nil {
		t.Fatalf("CaptureImage: %v", err)
	}
	got, err := os.ReadFile(image)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(src) {
		t.Fatalf("captured size = %d, want %d", len(got), len(src))
	}
	for i := range got {
		switch {
		case i < 17408 || i >= len(src)-16896:
			if got[i] != src[i] {
				t.Fatalf("GPT metadata byte %d mismatch", i)
			}
		case i >= partStart && i < partStart+len(allocated)*1024:
			block := (i - partStart) / 1024
			if allocated[block] {
				if got[i] != src[i] {
					t.Fatalf("allocated byte %d mismatch", i)
				}
			} else if got[i] != 0 {
				t.Fatalf("unallocated byte %d = %x, want zero", i, got[i])
			}
		default:
			if got[i] != 0 {
				t.Fatalf("gap byte %d = %x, want zero", i, got[i])
			}
		}
	}
}

func TestInvalidGPTFallsBackToRawImage(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "disk.img")
	corona := filepath.Join(dir, "disk.corona")

	src, _, _ := gptExtFixtureImage()
	src[512+16] ^= 0xff
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	if err := create(context.Background(), createOptions{
		SourcePath: image,
		CoronaPath: corona,
		ChunkSize:  4096,
		Workers:    2,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	info, err := validateCoronaForTest(t, corona)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	sum := sha256.Sum256(src)
	if info.AllocatedSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("AllocatedSHA256 = %s, want full raw image hash", info.AllocatedSHA256)
	}
}

func gptExtFixtureImage() ([]byte, int, []bool) {
	ext, allocated := extFixtureImage()
	const partStart = 1 << 20
	backupBytes := 512 + 128*128
	size := partStart + len(ext) + 256*1024 + backupBytes
	size = ((size + 511) / 512) * 512
	data := make([]byte, size)
	copy(data[partStart:], ext)

	entryCount := uint32(128)
	entrySize := uint32(128)
	entryBytes := int(entryCount * entrySize)
	entryLBA := uint64(2)
	backupLBA := uint64(size/512 - 1)
	partFirstLBA := uint64(partStart / 512)
	partLastLBA := uint64((partStart+len(ext))/512 - 1)

	data[510] = 0x55
	data[511] = 0xaa
	entry := data[entryLBA*512 : entryLBA*512+uint64(entryBytes)]
	entry[0] = 0x01
	binary.LittleEndian.PutUint64(entry[32:40], partFirstLBA)
	binary.LittleEndian.PutUint64(entry[40:48], partLastLBA)
	entriesCRC := crc32.ChecksumIEEE(data[entryLBA*512 : entryLBA*512+uint64(entryBytes)])
	writeGPTHeader(data[512:1024], 1, backupLBA, entryLBA, entryCount, entrySize, entriesCRC)

	backupEntryOffset := int(backupLBA*512) - entryBytes
	copy(data[backupEntryOffset:], data[entryLBA*512:entryLBA*512+uint64(entryBytes)])
	writeGPTHeader(data[backupLBA*512:backupLBA*512+512], backupLBA, 1, uint64(backupEntryOffset/512), entryCount, entrySize, entriesCRC)
	return data, partStart, allocated
}

func writeGPTHeader(header []byte, currentLBA, backupLBA, entryLBA uint64, entryCount, entrySize uint32, entriesCRC uint32) {
	copy(header[:8], []byte("EFI PART"))
	binary.LittleEndian.PutUint32(header[8:12], 0x00010000)
	binary.LittleEndian.PutUint32(header[12:16], 92)
	binary.LittleEndian.PutUint64(header[24:32], currentLBA)
	binary.LittleEndian.PutUint64(header[32:40], backupLBA)
	binary.LittleEndian.PutUint64(header[40:48], 34)
	binary.LittleEndian.PutUint64(header[48:56], backupLBA-34)
	binary.LittleEndian.PutUint64(header[72:80], entryLBA)
	binary.LittleEndian.PutUint32(header[80:84], entryCount)
	binary.LittleEndian.PutUint32(header[84:88], entrySize)
	binary.LittleEndian.PutUint32(header[88:92], entriesCRC)
	headerForCRC := make([]byte, 92)
	copy(headerForCRC, header[:92])
	clear(headerForCRC[16:20])
	binary.LittleEndian.PutUint32(header[16:20], crc32.ChecksumIEEE(headerForCRC))
}

func fillSkippableRangesWithGarbage(data []byte, partStart int, allocated []bool) {
	for i := 17408; i < len(data)-16896; i++ {
		if i >= partStart && i < partStart+len(allocated)*1024 {
			continue
		}
		data[i] = byte((i * 7) % 251)
	}
	for block, isAllocated := range allocated {
		if isAllocated {
			continue
		}
		start := partStart + block*1024
		for i := start; i < start+1024; i++ {
			data[i] = byte((i * 11) % 251)
		}
	}
}
