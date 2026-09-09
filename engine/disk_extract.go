package engine

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"unicode/utf16"

	"golang.org/x/sys/unix"

	"github.com/telemetryos/starforge/actions"
)

const diskSectorSize = uint64(512)

type diskPartition struct {
	name   string
	start  uint64
	length uint64
}

// ExtractDiskPartitions copies named GPT partitions from a sealed raw disk
// without mounting it or creating loop devices.
func ExtractDiskPartitions(diskPath string, definitions []actions.PartitionDef, outputDir string) ([]string, error) {
	disk, err := os.Open(diskPath)
	if err != nil {
		return nil, err
	}
	defer disk.Close()
	partitions, err := readGPTPartitions(disk)
	if err != nil {
		return nil, err
	}
	if len(partitions) != len(definitions) {
		return nil, fmt.Errorf("artifact has %d GPT partitions, expected %d", len(partitions), len(definitions))
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(definitions))
	for index, definition := range definitions {
		partition := partitions[index]
		if partition.name != definition.Name {
			return nil, fmt.Errorf("artifact partition %d is %q, expected %q", index+1, partition.name, definition.Name)
		}
		path := filepath.Join(outputDir, definition.Name+".img")
		if err := copySparseRange(disk, path, partition.start, partition.length); err != nil {
			return nil, fmt.Errorf("extracting partition %q: %w", definition.Name, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// CopySparseFile copies a regular file while preserving sparse holes when the
// source filesystem supports SEEK_DATA and SEEK_HOLE.
func CopySparseFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	return copySparseRange(input, destination, 0, uint64(info.Size()))
}

func readGPTPartitions(disk *os.File) ([]diskPartition, error) {
	info, err := disk.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < int64(2*diskSectorSize) {
		return nil, fmt.Errorf("artifact is too small for GPT")
	}
	header := make([]byte, diskSectorSize)
	if _, err := disk.ReadAt(header, int64(diskSectorSize)); err != nil {
		return nil, err
	}
	if string(header[:8]) != "EFI PART" {
		return nil, fmt.Errorf("artifact does not contain a GPT")
	}
	headerSize := binary.LittleEndian.Uint32(header[12:16])
	if headerSize < 92 || headerSize > uint32(len(header)) {
		return nil, fmt.Errorf("invalid GPT header size %d", headerSize)
	}
	expectedHeaderCRC := binary.LittleEndian.Uint32(header[16:20])
	headerForCRC := append([]byte(nil), header[:headerSize]...)
	clear(headerForCRC[16:20])
	if crc32.ChecksumIEEE(headerForCRC) != expectedHeaderCRC {
		return nil, fmt.Errorf("invalid GPT header checksum")
	}
	entryLBA := binary.LittleEndian.Uint64(header[72:80])
	entryCount := binary.LittleEndian.Uint32(header[80:84])
	entrySize := binary.LittleEndian.Uint32(header[84:88])
	if entryCount == 0 || entrySize < 128 || entrySize > 4096 {
		return nil, fmt.Errorf("invalid GPT entry table")
	}
	entryBytes := uint64(entryCount) * uint64(entrySize)
	entryOffset := entryLBA * diskSectorSize
	if entryOffset > uint64(info.Size()) || entryBytes > uint64(info.Size())-entryOffset {
		return nil, fmt.Errorf("GPT entry table exceeds artifact")
	}
	entries := make([]byte, entryBytes)
	if _, err := disk.ReadAt(entries, int64(entryOffset)); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(entries) != binary.LittleEndian.Uint32(header[88:92]) {
		return nil, fmt.Errorf("invalid GPT entry checksum")
	}
	partitions := make([]diskPartition, 0)
	for index := uint32(0); index < entryCount; index++ {
		entry := entries[uint64(index)*uint64(entrySize) : uint64(index+1)*uint64(entrySize)]
		if emptyBytes(entry[:16]) {
			continue
		}
		first := binary.LittleEndian.Uint64(entry[32:40])
		last := binary.LittleEndian.Uint64(entry[40:48])
		if last < first || last == ^uint64(0) || last+1 > uint64(info.Size())/diskSectorSize {
			return nil, fmt.Errorf("invalid GPT partition %d bounds", index+1)
		}
		partitions = append(partitions, diskPartition{
			name:   decodeGPTName(entry[56:128]),
			start:  first * diskSectorSize,
			length: (last - first + 1) * diskSectorSize,
		})
	}
	return partitions, nil
}

func decodeGPTName(data []byte) string {
	codepoints := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		value := binary.LittleEndian.Uint16(data[index : index+2])
		if value == 0 {
			break
		}
		codepoints = append(codepoints, value)
	}
	return string(utf16.Decode(codepoints))
}

func emptyBytes(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}

func copySparseRange(source *os.File, destination string, start, length uint64) error {
	if length > uint64(^uint64(0)>>1) {
		return fmt.Errorf("copy length exceeds host file limits")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary := destination + ".creating"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		output.Close()
		_ = os.Remove(temporary)
	}()
	if err := output.Truncate(int64(length)); err != nil {
		return err
	}
	if start > ^uint64(0)-length {
		return fmt.Errorf("copy range exceeds host file limits")
	}
	end := start + length
	cursor := start
	for cursor < end {
		dataOffset, seekErr := unix.Seek(int(source.Fd()), int64(cursor), unix.SEEK_DATA)
		if seekErr == unix.ENXIO {
			break
		}
		if seekErr == unix.EINVAL || seekErr == unix.ENOTSUP {
			if _, err := io.Copy(io.NewOffsetWriter(output, 0), io.NewSectionReader(source, int64(start), int64(length))); err != nil {
				return err
			}
			cursor = end
			break
		}
		if seekErr != nil {
			return seekErr
		}
		if uint64(dataOffset) >= end {
			break
		}
		holeOffset, seekErr := unix.Seek(int(source.Fd()), dataOffset, unix.SEEK_HOLE)
		if seekErr != nil {
			return seekErr
		}
		dataEnd := uint64(holeOffset)
		if dataEnd > end {
			dataEnd = end
		}
		copyLength := dataEnd - uint64(dataOffset)
		reader := io.NewSectionReader(source, dataOffset, int64(copyLength))
		writer := io.NewOffsetWriter(output, dataOffset-int64(start))
		if _, err := io.Copy(writer, reader); err != nil {
			return err
		}
		cursor = dataEnd
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return err
	}
	return nil
}
