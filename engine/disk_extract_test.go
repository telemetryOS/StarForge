package engine

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/telemetryos/starforge/actions"
)

func TestExtractDiskPartitions(t *testing.T) {
	directory := t.TempDir()
	diskPath := filepath.Join(directory, "disk.raw")
	partitions := []diskPartition{
		{name: "boot", start: 2048 * diskSectorSize, length: 8 * diskSectorSize},
		{name: "root", start: 4096 * diskSectorSize, length: 16 * diskSectorSize},
	}
	writeTestGPT(t, diskPath, 8<<20, partitions)
	disk, err := os.OpenFile(diskPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disk.WriteAt([]byte("BOOT"), int64(partitions[0].start)); err != nil {
		t.Fatal(err)
	}
	if _, err := disk.WriteAt([]byte("ROOT"), int64(partitions[1].start)); err != nil {
		t.Fatal(err)
	}
	if err := disk.Close(); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(directory, "parts")
	paths, err := ExtractDiskPartitions(diskPath, []actions.PartitionDef{{Name: "boot"}, {Name: "root"}}, outputDir)
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []string{"BOOT", "ROOT"} {
		data, err := os.ReadFile(paths[index])
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != int(partitions[index].length) || string(data[:4]) != want {
			t.Fatalf("partition %d has size %d and prefix %q", index, len(data), data[:4])
		}
	}
}

func TestExtractDiskPartitionsRejectsNameMismatch(t *testing.T) {
	directory := t.TempDir()
	diskPath := filepath.Join(directory, "disk.raw")
	writeTestGPT(t, diskPath, 4<<20, []diskPartition{{name: "root", start: 2048 * diskSectorSize, length: 8 * diskSectorSize}})
	_, err := ExtractDiskPartitions(diskPath, []actions.PartitionDef{{Name: "data"}}, filepath.Join(directory, "parts"))
	if err == nil {
		t.Fatal("partition name mismatch unexpectedly succeeded")
	}
}

func writeTestGPT(t *testing.T, path string, size int64, partitions []diskPartition) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	entries := make([]byte, 128*128)
	for index, partition := range partitions {
		entry := entries[index*128 : (index+1)*128]
		entry[0] = 1
		entry[16] = byte(index + 1)
		binary.LittleEndian.PutUint64(entry[32:40], partition.start/diskSectorSize)
		binary.LittleEndian.PutUint64(entry[40:48], (partition.start+partition.length)/diskSectorSize-1)
		name := utf16.Encode([]rune(partition.name))
		for offset, value := range name {
			binary.LittleEndian.PutUint16(entry[56+offset*2:], value)
		}
	}
	if _, err := file.WriteAt(entries, int64(2*diskSectorSize)); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, diskSectorSize)
	copy(header, "EFI PART")
	binary.LittleEndian.PutUint32(header[8:12], 0x00010000)
	binary.LittleEndian.PutUint32(header[12:16], 92)
	binary.LittleEndian.PutUint64(header[24:32], 1)
	lastLBA := uint64(size)/diskSectorSize - 1
	binary.LittleEndian.PutUint64(header[32:40], lastLBA)
	binary.LittleEndian.PutUint64(header[40:48], 34)
	binary.LittleEndian.PutUint64(header[48:56], lastLBA-34)
	header[56] = 1
	binary.LittleEndian.PutUint64(header[72:80], 2)
	binary.LittleEndian.PutUint32(header[80:84], 128)
	binary.LittleEndian.PutUint32(header[84:88], 128)
	binary.LittleEndian.PutUint32(header[88:92], crc32.ChecksumIEEE(entries))
	binary.LittleEndian.PutUint32(header[16:20], crc32.ChecksumIEEE(header[:92]))
	if _, err := file.WriteAt(header, int64(diskSectorSize)); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
