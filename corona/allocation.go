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
	if checker, err := detectExtAllocationChecker(src, imageSize); err == nil && checker != nil {
		return checker
	}
	if checker, err := detectFATAllocationChecker(src, imageSize); err == nil && checker != nil {
		return checker
	}
	return nil
}

func contiguousAllocationPlan(offset, maxSize, fsSize uint64, allocatedAt func(uint64) (bool, error), nextBoundary func(uint64) uint64) (framePlan, error) {
	if maxSize == 0 {
		return framePlan{}, errEmptyFramePlan
	}
	if offset >= fsSize {
		return framePlan{offset: offset, size: maxSize}, nil
	}
	allocated, err := allocatedAt(offset)
	if err != nil {
		return framePlan{}, err
	}
	skip := !allocated
	end := offset
	for end-offset < maxSize && end < fsSize {
		isAllocated, err := allocatedAt(end)
		if err != nil {
			return framePlan{}, err
		}
		if !isAllocated != skip {
			break
		}
		next := nextBoundary(end)
		if next > fsSize {
			next = fsSize
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
