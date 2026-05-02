package corona

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const blkGetSize64 = 0x80081272

func validateTargetCapacity(target *os.File, imageSize uint64) error {
	size, ok, err := targetCapacity(target)
	if err != nil {
		return err
	}
	if ok && size < imageSize {
		return fmt.Errorf("corona: target too small: size=%d image=%d", size, imageSize)
	}
	return nil
}

func targetCapacity(target *os.File) (uint64, bool, error) {
	info, err := target.Stat()
	if err != nil {
		return 0, false, fmt.Errorf("stat target: %w", err)
	}
	mode := info.Mode()
	if mode.IsRegular() {
		return uint64(info.Size()), true, nil
	}
	if mode&os.ModeDevice == 0 || mode&os.ModeCharDevice != 0 {
		return 0, false, nil
	}
	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, target.Fd(), uintptr(blkGetSize64), uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, false, fmt.Errorf("get target block size: %w", errno)
	}
	return size, true, nil
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
