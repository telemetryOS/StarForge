package corona

import (
	"encoding/binary"
	"fmt"
	"os"
)

type fatAllocationChecker struct {
	src               *os.File
	version           uint16
	fsSize            uint64
	bytesPerSector    uint64
	sectorsPerCluster uint64
	clusterSize       uint64
	fatOffset         uint64
	dataOffset        uint64
	clusterCount      uint64
}

func detectFATAllocationChecker(src *os.File, imageSize uint64) (*fatAllocationChecker, error) {
	if imageSize < 512 {
		return nil, nil
	}
	var boot [512]byte
	if _, err := src.ReadAt(boot[:], 0); err != nil {
		return nil, err
	}
	if boot[510] != 0x55 || boot[511] != 0xaa {
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
	default:
		version = 32
	}
	fsSize := totalSectors * bytesPerSector
	if fsSize > imageSize {
		return nil, nil
	}
	return &fatAllocationChecker{
		src:               src,
		version:           version,
		fsSize:            fsSize,
		bytesPerSector:    bytesPerSector,
		sectorsPerCluster: sectorsPerCluster,
		clusterSize:       bytesPerSector * sectorsPerCluster,
		fatOffset:         reservedSectors * bytesPerSector,
		dataOffset:        dataStartSector * bytesPerSector,
		clusterCount:      clusterCount,
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
	if offset >= c.fsSize {
		return framePlan{offset: offset, size: maxSize}, nil
	}
	if offset < c.dataOffset {
		return framePlan{offset: offset, size: minUint64(maxSize, c.dataOffset-offset)}, nil
	}
	return contiguousAllocationPlan(
		offset,
		maxSize,
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
		var entry [2]byte
		if _, err := c.src.ReadAt(entry[:], int64(c.fatOffset+cluster*2)); err != nil {
			return false, fmt.Errorf("read fat16 entry %d: %w", cluster, err)
		}
		return binary.LittleEndian.Uint16(entry[:]) != 0, nil
	case 32:
		var entry [4]byte
		if _, err := c.src.ReadAt(entry[:], int64(c.fatOffset+cluster*4)); err != nil {
			return false, fmt.Errorf("read fat32 entry %d: %w", cluster, err)
		}
		return binary.LittleEndian.Uint32(entry[:])&0x0fffffff != 0, nil
	default:
		return true, nil
	}
}
