package corona

type rawAllocationChecker struct {
	baseOffset uint64
	size       uint64
}

func (c rawAllocationChecker) header() fileHeader {
	return fileHeader{}
}

func (c rawAllocationChecker) nextFramePlan(offset, maxSize uint64) (framePlan, error) {
	if maxSize == 0 {
		return framePlan{}, errEmptyFramePlan
	}
	end := c.baseOffset + c.size
	if offset >= end {
		return framePlan{offset: offset, size: maxSize}, nil
	}
	return framePlan{offset: offset, size: minUint64(maxSize, end-offset)}, nil
}
