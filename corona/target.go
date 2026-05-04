package corona

import (
	"fmt"
	"os"
)

func validateTargetCapacity(target *os.File, imageSize uint64) error {
	size, err := targetCapacity(target)
	if err != nil {
		return err
	}
	if size < imageSize {
		return fmt.Errorf("corona: target too small: size=%d image=%d", size, imageSize)
	}
	return nil
}

func writeZeros(target *os.File, offset, size uint64) error {
	for size > 0 {
		n := uint64(len(zeroBlock))
		if size < n {
			n = size
		}
		if _, err := target.WriteAt(zeroBlock[:n], int64(offset)); err != nil {
			return fmt.Errorf("zero range at %d: %w", offset, err)
		}
		offset += n
		size -= n
	}
	return nil
}
