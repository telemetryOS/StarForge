package corona

import (
	"encoding/binary"
	"fmt"
	"os"
)

type fatAllocationChecker struct {
	src               *os.File
	baseOffset        uint64
	version           uint16
	fsSize            uint64
	bytesPerSector    uint64
	sectorsPerCluster uint64
	clusterSize       uint64
	fatOffset         uint64
	dataOffset        uint64
	clusterCount      uint64
	fat               []byte
}

func detectFATAllocationChecker(src *os.File, imageSize uint64) (*fatAllocationChecker, error) {
	return detectFATAllocationCheckerAt(src, 0, imageSize)
}

func detectFATAllocationCheckerAt(src *os.File, baseOffset, imageSize uint64) (*fatAllocationChecker, error) {
	if imageSize < 512 {
		return nil, nil
	}
	var boot [512]byte
	if _, err := src.ReadAt(boot[:], int64(baseOffset)); err != nil {
		return nil, err
	}
	if boot[510] != 0x55 || boot[511] != 0xaa {
		return nil, nil
	}
	if boot[0] != 0xeb && boot[0] != 0xe9 {
		return nil, nil
	}
	bytesPerSector := uint64(binary.LittleEndian.Uint16(boot[11:13]))
	sectorsPerCluster := uint64(boot[13])
	reservedSectors := uint64(binary.LittleEndian.Uint16(boot[14:16]))
	fatCount := uint64(boot[16])
	rootEntryCount := uint64(binary.LittleEndian.Uint16(boot[17:19]))
	totalSectors := uint64(binary.LittleEndian.Uint16(boot[19:21]))
	if totalSectors == 0 {
		totalSectors = uint64(binary.LittleEndian.Uint32(boot[32:36]))
	}
	fatSectors := uint64(binary.LittleEndian.Uint16(boot[22:24]))
	if fatSectors == 0 {
		fatSectors = uint64(binary.LittleEndian.Uint32(boot[36:40]))
	}
	if bytesPerSector == 0 || sectorsPerCluster == 0 || reservedSectors == 0 || fatCount == 0 || totalSectors == 0 || fatSectors == 0 {
		return nil, nil
	}
	if bytesPerSector < 512 || bytesPerSector > 4096 || bytesPerSector&(bytesPerSector-1) != 0 {
		return nil, nil
	}
	if sectorsPerCluster&(sectorsPerCluster-1) != 0 {
		return nil, nil
	}

	rootDirSectors := ((rootEntryCount * 32) + (bytesPerSector - 1)) / bytesPerSector
	dataStartSector := reservedSectors + fatCount*fatSectors + rootDirSectors
	if dataStartSector >= totalSectors {
		return nil, nil
	}
	clusterCount := (totalSectors - dataStartSector) / sectorsPerCluster
	var version uint16
	switch {
	case clusterCount < 4085:
		return nil, nil
	case clusterCount < 65525:
		version = 16
		if string(boot[54:62]) != "FAT16   " {
			return nil, nil
		}
	default:
		version = 32
		if string(boot[82:90]) != "FAT32   " {
			return nil, nil
		}
	}
	fsSize := totalSectors * bytesPerSector
	if fsSize > imageSize {
		return nil, nil
	}
	fatBytes := fatSectors * bytesPerSector
	fat := make([]byte, fatBytes)
	if _, err := src.ReadAt(fat, int64(baseOffset+reservedSectors*bytesPerSector)); err != nil {
		return nil, err
	}
	return &fatAllocationChecker{
		src:               src,
		baseOffset:        baseOffset,
		version:           version,
		fsSize:            fsSize,
		bytesPerSector:    bytesPerSector,
		sectorsPerCluster: sectorsPerCluster,
		clusterSize:       bytesPerSector * sectorsPerCluster,
		fatOffset:         reservedSectors * bytesPerSector,
		dataOffset:        dataStartSector * bytesPerSector,
		clusterCount:      clusterCount,
		fat:               fat,
	}, nil
}

func (c *fatAllocationChecker) header() fileHeader {
	return fileHeader{
		fsType:      fsFAT,
		fsVersion:   c.version,
		fsBlockSize: c.clusterSize,
	}
}

func (c *fatAllocationChecker) nextFramePlan(offset, maxSize uint64) (framePlan, error) {
	if maxSize == 0 {
		return framePlan{}, errEmptyFramePlan
	}
	if offset >= c.baseOffset+c.fsSize {
		return framePlan{offset: offset, size: maxSize}, nil
	}
	localOffset := offset - c.baseOffset
	if localOffset < c.dataOffset {
		return framePlan{offset: offset, size: minUint64(maxSize, c.dataOffset-localOffset)}, nil
	}
	return contiguousAllocationPlan(
		offset,
		maxSize,
		c.baseOffset,
		c.fsSize,
		func(offset uint64) (bool, error) {
			return c.clusterAllocated(c.clusterForOffset(offset))
		},
		func(offset uint64) uint64 {
			return c.offsetForCluster(c.clusterForOffset(offset) + 1)
		},
	)
}

func (c *fatAllocationChecker) clusterForOffset(offset uint64) uint64 {
	return 2 + (offset-c.dataOffset)/c.clusterSize
}

func (c *fatAllocationChecker) offsetForCluster(cluster uint64) uint64 {
	return c.dataOffset + (cluster-2)*c.clusterSize
}

func (c *fatAllocationChecker) clusterAllocated(cluster uint64) (bool, error) {
	if cluster < 2 || cluster >= c.clusterCount+2 {
		return true, nil
	}
	switch c.version {
	case 16:
		entryOffset := cluster * 2
		if entryOffset+2 > uint64(len(c.fat)) {
			return false, fmt.Errorf("corona: fat16 entry %d outside FAT", cluster)
		}
		return binary.LittleEndian.Uint16(c.fat[entryOffset:entryOffset+2]) != 0, nil
	case 32:
		entryOffset := cluster * 4
		if entryOffset+4 > uint64(len(c.fat)) {
			return false, fmt.Errorf("corona: fat32 entry %d outside FAT", cluster)
		}
		return binary.LittleEndian.Uint32(c.fat[entryOffset:entryOffset+4])&0x0fffffff != 0, nil
	default:
		return true, nil
	}
}
