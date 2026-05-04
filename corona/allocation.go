package corona

import (
	"errors"
	"os"
)

type allocationChecker interface {
	header() fileHeader
	nextFramePlan(offset, maxSize uint64) (framePlan, error)
}

type framePlan struct {
	offset uint64
	size   uint64
	skip   bool
}

var errEmptyFramePlan = errors.New("corona: empty pack frame")

func detectAllocationChecker(src *os.File, imageSize uint64) allocationChecker {
	if checker, err := detectGPTAllocationChecker(src, imageSize); err == nil && checker != nil {
		return checker
	}
	if checker, err := detectExtAllocationCheckerAt(src, 0, imageSize); err == nil && checker != nil {
		return checker
	}
	if checker, err := detectFATAllocationCheckerAt(src, 0, imageSize); err == nil && checker != nil {
		return checker
	}
	return nil
}

func contiguousAllocationPlan(offset, maxSize, baseOffset, fsSize uint64, allocatedAt func(uint64) (bool, error), nextBoundary func(uint64) uint64) (framePlan, error) {
	if maxSize == 0 {
		return framePlan{}, errEmptyFramePlan
	}
	fsEnd := baseOffset + fsSize
	if offset >= fsEnd {
		return framePlan{offset: offset, size: maxSize, skip: true}, nil
	}
	localOffset := offset - baseOffset
	allocated, err := allocatedAt(localOffset)
	if err != nil {
		return framePlan{}, err
	}
	skip := !allocated
	end := offset
	for end-offset < maxSize && end < fsEnd {
		localEnd := end - baseOffset
		isAllocated, err := allocatedAt(localEnd)
		if err != nil {
			return framePlan{}, err
		}
		if !isAllocated != skip {
			break
		}
		next := baseOffset + nextBoundary(localEnd)
		if next > fsEnd {
			next = fsEnd
		}
		if next-offset > maxSize {
			next = offset + maxSize
		}
		end = next
	}
	if end == offset {
		end = offset + maxSize
	}
	return framePlan{offset: offset, size: end - offset, skip: skip}, nil
}
