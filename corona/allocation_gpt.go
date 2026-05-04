package corona

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"sort"
)

const gptSectorSize = 512

type diskRange struct {
	start uint64
	end   uint64
}

type gptPartition struct {
	start   uint64
	end     uint64
	planner allocationChecker
}

type gptAllocationChecker struct {
	src        *os.File
	imageSize  uint64
	metadata   []diskRange
	partitions []gptPartition
}

func detectGPTAllocationChecker(src *os.File, imageSize uint64) (*gptAllocationChecker, error) {
	if imageSize < 2*gptSectorSize {
		return nil, nil
	}
	var header [gptSectorSize]byte
	if _, err := src.ReadAt(header[:], gptSectorSize); err != nil {
		return nil, err
	}
	if string(header[:8]) != "EFI PART" {
		return nil, nil
	}
	headerSize := uint64(binary.LittleEndian.Uint32(header[12:16]))
	if headerSize < 92 || headerSize > gptSectorSize {
		return nil, nil
	}
	expectedHeaderCRC := binary.LittleEndian.Uint32(header[16:20])
	headerForCRC := make([]byte, headerSize)
	copy(headerForCRC, header[:headerSize])
	clear(headerForCRC[16:20])
	if crc32.ChecksumIEEE(headerForCRC) != expectedHeaderCRC {
		return nil, nil
	}
	currentLBA := binary.LittleEndian.Uint64(header[24:32])
	backupLBA := binary.LittleEndian.Uint64(header[32:40])
	firstUsableLBA := binary.LittleEndian.Uint64(header[40:48])
	lastUsableLBA := binary.LittleEndian.Uint64(header[48:56])
	entryLBA := binary.LittleEndian.Uint64(header[72:80])
	entryCount := uint64(binary.LittleEndian.Uint32(header[80:84]))
	entrySize := uint64(binary.LittleEndian.Uint32(header[84:88]))
	expectedEntriesCRC := binary.LittleEndian.Uint32(header[88:92])
	lastDiskLBA := imageSize/gptSectorSize - 1
	if currentLBA != 1 || backupLBA > lastDiskLBA || firstUsableLBA > lastUsableLBA || lastUsableLBA > lastDiskLBA {
		return nil, nil
	}
	if entryCount == 0 || entrySize < 128 || entrySize > 4096 || entrySize%8 != 0 {
		return nil, nil
	}
	entryBytes := entryCount * entrySize
	entryOffset := entryLBA * gptSectorSize
	if entryOffset >= imageSize || entryBytes > imageSize-entryOffset {
		return nil, nil
	}
	entries := make([]byte, entryBytes)
	if _, err := src.ReadAt(entries, int64(entryOffset)); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(entries) != expectedEntriesCRC {
		return nil, nil
	}
	partitions := make([]gptPartition, 0)
	for i := uint64(0); i < entryCount; i++ {
		entry := entries[i*entrySize : (i+1)*entrySize]
		if emptyGUID(entry[:16]) {
			continue
		}
		firstLBA := binary.LittleEndian.Uint64(entry[32:40])
		lastLBA := binary.LittleEndian.Uint64(entry[40:48])
		if firstLBA < firstUsableLBA || lastLBA > lastUsableLBA || lastLBA < firstLBA {
			continue
		}
		start := firstLBA * gptSectorSize
		end := (lastLBA + 1) * gptSectorSize
		if start >= imageSize {
			continue
		}
		if end > imageSize {
			end = imageSize
		}
		partitions = append(partitions, gptPartition{start: start, end: end})
	}
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].start < partitions[j].start
	})

	metadata := []diskRange{{start: 0, end: gptSectorSize}, {start: gptSectorSize, end: entryOffset + entryBytes}}
	backupHeader := backupLBA * gptSectorSize
	if backupHeader < imageSize {
		backupStart := backupHeader
		if entryBytes < backupHeader {
			backupStart = backupHeader - entryBytes
		}
		metadata = append(metadata, diskRange{start: backupStart, end: minUint64(imageSize, backupHeader+gptSectorSize)})
	}
	metadata = mergeDiskRanges(metadata, imageSize)
	if !validGPTPartitions(partitions, metadata) {
		return nil, nil
	}

	return &gptAllocationChecker{
		src:        src,
		imageSize:  imageSize,
		metadata:   metadata,
		partitions: partitions,
	}, nil
}

func validGPTPartitions(partitions []gptPartition, metadata []diskRange) bool {
	var lastEnd uint64
	for i, p := range partitions {
		if p.end <= p.start {
			return false
		}
		if i > 0 && p.start < lastEnd {
			return false
		}
		for _, r := range metadata {
			if p.start < r.end && p.end > r.start {
				return false
			}
		}
		lastEnd = p.end
	}
	return true
}

func (c *gptAllocationChecker) header() fileHeader {
	return fileHeader{fsType: fsUnknown}
}

func (c *gptAllocationChecker) nextFramePlan(offset, maxSize uint64) (framePlan, error) {
	if maxSize == 0 {
		return framePlan{}, errEmptyFramePlan
	}
	if offset >= c.imageSize {
		return framePlan{offset: offset, size: maxSize}, nil
	}
	limit := minUint64(c.imageSize, offset+maxSize)
	if r, ok := rangeContaining(c.metadata, offset); ok {
		return framePlan{offset: offset, size: minUint64(limit, r.end) - offset}, nil
	}
	for i := range c.partitions {
		p := &c.partitions[i]
		if offset < p.start {
			next := minUint64(limit, p.start)
			next = minNextRangeStart(c.metadata, offset, next)
			return framePlan{offset: offset, size: next - offset, skip: true}, nil
		}
		if offset >= p.start && offset < p.end {
			if p.planner == nil {
				p.planner = c.detectPartitionPlanner(p.start, p.end-p.start)
			}
			planMax := minUint64(maxSize, p.end-offset)
			return p.planner.nextFramePlan(offset, planMax)
		}
	}
	next := limit
	next = minNextRangeStart(c.metadata, offset, next)
	return framePlan{offset: offset, size: next - offset, skip: true}, nil
}

func (c *gptAllocationChecker) detectPartitionPlanner(start, size uint64) allocationChecker {
	if checker, err := detectExtAllocationCheckerAt(c.src, start, size); err == nil && checker != nil {
		return checker
	}
	if checker, err := detectFATAllocationCheckerAt(c.src, start, size); err == nil && checker != nil {
		return checker
	}
	return rawAllocationChecker{baseOffset: start, size: size}
}

func emptyGUID(guid []byte) bool {
	for _, b := range guid {
		if b != 0 {
			return false
		}
	}
	return true
}

func mergeDiskRanges(ranges []diskRange, imageSize uint64) []diskRange {
	out := ranges[:0]
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	for _, r := range ranges {
		if r.start >= imageSize {
			continue
		}
		if r.end > imageSize {
			r.end = imageSize
		}
		if r.end <= r.start {
			continue
		}
		if len(out) > 0 && r.start <= out[len(out)-1].end {
			if r.end > out[len(out)-1].end {
				out[len(out)-1].end = r.end
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func rangeContaining(ranges []diskRange, offset uint64) (diskRange, bool) {
	for _, r := range ranges {
		if offset >= r.start && offset < r.end {
			return r, true
		}
	}
	return diskRange{}, false
}

func minNextRangeStart(ranges []diskRange, offset, limit uint64) uint64 {
	next := limit
	for _, r := range ranges {
		if r.start > offset && r.start < next {
			next = r.start
		}
	}
	return next
}
