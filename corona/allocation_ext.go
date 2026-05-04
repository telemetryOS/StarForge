package corona

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type extAllocationChecker struct {
	src            *os.File
	baseOffset     uint64
	fsType         uint8
	fsVersion      uint16
	fsBlockSize    uint64
	fsSize         uint64
	firstDataBlock uint64
	blocksCount    uint64
	blocksPerGroup uint64
	descSize       uint64
	gdtOffset      uint64
	bitmaps        map[uint64][]byte
}

func detectExtAllocationChecker(src *os.File, imageSize uint64) (*extAllocationChecker, error) {
	return detectExtAllocationCheckerAt(src, 0, imageSize)
}

func detectExtAllocationCheckerAt(src *os.File, baseOffset, imageSize uint64) (*extAllocationChecker, error) {
	if imageSize < 2048 {
		return nil, nil
	}
	var super [1024]byte
	if _, err := src.ReadAt(super[:], int64(baseOffset+1024)); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint16(super[56:58]) != 0xef53 {
		return nil, nil
	}

	logBlockSize := binary.LittleEndian.Uint32(super[24:28])
	if logBlockSize > 16 {
		return nil, fmt.Errorf("corona: unsupported ext block size shift %d", logBlockSize)
	}
	blockSize := uint64(1024) << logBlockSize
	if blockSize < 1024 || blockSize > 1<<20 {
		return nil, fmt.Errorf("corona: unsupported ext block size %d", blockSize)
	}
	blocksPerGroup := uint64(binary.LittleEndian.Uint32(super[32:36]))
	if blocksPerGroup == 0 {
		return nil, errors.New("corona: ext blocks per group is zero")
	}
	blocksCount := uint64(binary.LittleEndian.Uint32(super[4:8]))
	if len(super) >= 344 {
		blocksCount |= uint64(binary.LittleEndian.Uint32(super[336:340])) << 32
	}
	if blocksCount == 0 {
		return nil, errors.New("corona: ext block count is zero")
	}
	fsSize := blocksCount * blockSize
	if fsSize > imageSize {
		return nil, fmt.Errorf("corona: ext filesystem size %d exceeds image size %d", fsSize, imageSize)
	}

	descSize := uint64(binary.LittleEndian.Uint16(super[254:256]))
	if descSize < 32 {
		descSize = 32
	}
	if descSize > blockSize {
		return nil, fmt.Errorf("corona: ext group descriptor size %d exceeds block size %d", descSize, blockSize)
	}
	firstDataBlock := uint64(binary.LittleEndian.Uint32(super[20:24]))
	gdtOffset := (firstDataBlock + 1) * blockSize
	groupCount := (blocksCount + blocksPerGroup - 1) / blocksPerGroup
	if groupCount > 1<<24 {
		return nil, fmt.Errorf("corona: ext group count %d is unreasonable", groupCount)
	}

	return &extAllocationChecker{
		src:            src,
		baseOffset:     baseOffset,
		fsType:         fsExt,
		fsVersion:      4,
		fsBlockSize:    blockSize,
		fsSize:         fsSize,
		firstDataBlock: firstDataBlock,
		blocksCount:    blocksCount,
		blocksPerGroup: blocksPerGroup,
		descSize:       descSize,
		gdtOffset:      gdtOffset,
		bitmaps:        make(map[uint64][]byte),
	}, nil
}

func (c *extAllocationChecker) header() fileHeader {
	return fileHeader{
		fsType:      c.fsType,
		fsVersion:   c.fsVersion,
		fsBlockSize: c.fsBlockSize,
	}
}

func (c *extAllocationChecker) nextFramePlan(offset, maxSize uint64) (framePlan, error) {
	return contiguousAllocationPlan(
		offset,
		maxSize,
		c.baseOffset,
		c.fsSize,
		func(offset uint64) (bool, error) {
			return c.blockAllocated(offset / c.fsBlockSize)
		},
		func(offset uint64) uint64 {
			block := offset / c.fsBlockSize
			return (block + 1) * c.fsBlockSize
		},
	)
}

func (c *extAllocationChecker) blockAllocated(block uint64) (bool, error) {
	if block < c.firstDataBlock {
		return true, nil
	}
	logicalBlock := block - c.firstDataBlock
	if block >= c.blocksCount {
		return true, nil
	}
	group := logicalBlock / c.blocksPerGroup
	groupOffset := logicalBlock % c.blocksPerGroup
	bitmap, err := c.groupBitmap(group)
	if err != nil {
		return false, err
	}
	return bitmap[groupOffset/8]&(1<<(groupOffset%8)) != 0, nil
}

func (c *extAllocationChecker) groupBitmap(group uint64) ([]byte, error) {
	if bitmap := c.bitmaps[group]; bitmap != nil {
		return bitmap, nil
	}
	desc := make([]byte, c.descSize)
	if _, err := c.src.ReadAt(desc, int64(c.baseOffset+c.gdtOffset+group*c.descSize)); err != nil {
		return nil, fmt.Errorf("read ext group descriptor %d: %w", group, err)
	}
	bitmapBlock := uint64(binary.LittleEndian.Uint32(desc[0:4]))
	if c.descSize >= 64 {
		bitmapBlock |= uint64(binary.LittleEndian.Uint32(desc[32:36])) << 32
	}
	if bitmapBlock >= c.blocksCount {
		return nil, fmt.Errorf("corona: ext block bitmap %d outside filesystem", bitmapBlock)
	}
	bitmap := make([]byte, c.fsBlockSize)
	if _, err := c.src.ReadAt(bitmap, int64(c.baseOffset+bitmapBlock*c.fsBlockSize)); err != nil {
		return nil, fmt.Errorf("read ext block bitmap %d: %w", group, err)
	}
	c.bitmaps[group] = bitmap
	return bitmap, nil
}
