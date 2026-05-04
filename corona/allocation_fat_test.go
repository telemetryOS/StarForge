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

func TestCreateFATSkipsUnallocatedClusters(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "fat.img")
	corona := filepath.Join(dir, "fat.corona")
	target := filepath.Join(dir, "target.blk")

	src, metaBytes, allocatedClusters, clusterSize := fat16FixtureImage()
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	dirty := bytes.Repeat([]byte{0xcc}, len(src))
	if err := os.WriteFile(target, dirty, 0644); err != nil {
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
	if info.FSType != fsFAT || info.FSVersion != 16 || info.FSBlockSize != uint64(clusterSize) {
		t.Fatalf("filesystem metadata = type %d version %d block %d, want FAT16 block %d", info.FSType, info.FSVersion, info.FSBlockSize, clusterSize)
	}
	if info.AllocatedSHA256 != fatAllocatedSHA(src, metaBytes, allocatedClusters, clusterSize) {
		t.Fatalf("AllocatedSHA256 = %q, want FAT allocated-only hash", info.AllocatedSHA256)
	}
	if err := writeCoronaToRegularForTest(t, corona, target, 0, WriteOrderSequential); err != nil {
		t.Fatalf("Flash: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:metaBytes], src[:metaBytes]) {
		t.Fatal("FAT metadata range was not restored")
	}
	for cluster, isAllocated := range allocatedClusters {
		start := metaBytes + cluster*clusterSize
		end := start + clusterSize
		if isAllocated {
			if !bytes.Equal(got[start:end], src[start:end]) {
				t.Fatalf("allocated cluster %d was not restored", cluster)
			}
			continue
		}
		if !bytes.Equal(got[start:end], dirty[start:end]) {
			t.Fatalf("unallocated cluster %d was overwritten", cluster)
		}
	}
}

func TestFlashImageFATSkipsUnallocatedClusters(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "fat.img")
	target := filepath.Join(dir, "target.blk")

	src, metaBytes, allocatedClusters, clusterSize := fat16FixtureImage()
	if err := os.WriteFile(image, src, 0644); err != nil {
		t.Fatal(err)
	}
	dirty := bytes.Repeat([]byte{0xaa}, len(src))
	if err := os.WriteFile(target, dirty, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeImageToRegularForTest(t, image, target, 4096, 2, WriteOrderSequential); err != nil {
		t.Fatalf("FlashImage: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:metaBytes], src[:metaBytes]) {
		t.Fatal("FAT metadata range was not restored")
	}
	for cluster, isAllocated := range allocatedClusters {
		start := metaBytes + cluster*clusterSize
		end := start + clusterSize
		if isAllocated {
			if !bytes.Equal(got[start:end], src[start:end]) {
				t.Fatalf("allocated cluster %d was not restored", cluster)
			}
			continue
		}
		if !bytes.Equal(got[start:end], dirty[start:end]) {
			t.Fatalf("unallocated cluster %d was overwritten", cluster)
		}
	}
}

func fat16FixtureImage() ([]byte, int, []bool, int) {
	const bytesPerSector = 512
	const sectorsPerCluster = 1
	const clusterSize = bytesPerSector * sectorsPerCluster
	const clusters = 5000
	const reservedSectors = 1
	const fatCount = 1
	const rootEntries = 512
	const fatSectors = 20
	rootDirSectors := ((rootEntries * 32) + (bytesPerSector - 1)) / bytesPerSector
	dataStartSectors := reservedSectors + fatCount*fatSectors + rootDirSectors
	totalSectors := dataStartSectors + clusters*sectorsPerCluster
	data := make([]byte, totalSectors*bytesPerSector)
	allocated := make([]bool, clusters)
	for _, cluster := range []int{0, 1, 8, 4096} {
		allocated[cluster] = true
	}

	boot := data[:bytesPerSector]
	boot[0] = 0xeb
	boot[1] = 0x3c
	boot[2] = 0x90
	copy(boot[3:11], []byte("MSDOS5.0"))
	binary.LittleEndian.PutUint16(boot[11:13], bytesPerSector)
	boot[13] = sectorsPerCluster
	binary.LittleEndian.PutUint16(boot[14:16], reservedSectors)
	boot[16] = fatCount
	binary.LittleEndian.PutUint16(boot[17:19], rootEntries)
	binary.LittleEndian.PutUint16(boot[19:21], uint16(totalSectors))
	boot[21] = 0xf8
	binary.LittleEndian.PutUint16(boot[22:24], fatSectors)
	copy(boot[54:62], []byte("FAT16   "))
	boot[510] = 0x55
	boot[511] = 0xaa

	fat := data[reservedSectors*bytesPerSector : (reservedSectors+fatSectors)*bytesPerSector]
	binary.LittleEndian.PutUint16(fat[0:2], 0xfff8)
	binary.LittleEndian.PutUint16(fat[2:4], 0xffff)
	for cluster, isAllocated := range allocated {
		if isAllocated {
			entryOffset := (cluster + 2) * 2
			binary.LittleEndian.PutUint16(fat[entryOffset:entryOffset+2], 0xffff)
		}
	}

	metaBytes := dataStartSectors * bytesPerSector
	for cluster, isAllocated := range allocated {
		if !isAllocated {
			continue
		}
		start := metaBytes + cluster*clusterSize
		for i := 0; i < clusterSize; i++ {
			data[start+i] = byte((cluster*23 + i) % 251)
		}
	}
	return data, metaBytes, allocated, clusterSize
}

func fatAllocatedSHA(data []byte, metaBytes int, allocated []bool, clusterSize int) string {
	h := sha256.New()
	_, _ = h.Write(data[:metaBytes])
	for cluster, isAllocated := range allocated {
		if !isAllocated {
			continue
		}
		start := metaBytes + cluster*clusterSize
		_, _ = h.Write(data[start : start+clusterSize])
	}
	return hex.EncodeToString(h.Sum(nil))
}
