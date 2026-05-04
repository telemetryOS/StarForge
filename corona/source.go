package corona

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

func sourceCapacity(f *os.File) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat source: %w", err)
	}
	if isBlockDevice(info.Mode()) {
		var size uint64
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(unix.BLKGETSIZE64), uintptr(unsafe.Pointer(&size)))
		if errno != 0 {
			return 0, fmt.Errorf("get block device size: %w", errno)
		}
		return size, nil
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("corona: unsupported source type %s", info.Mode().Type())
	}
	if info.Size() <= 0 {
		return 0, fmt.Errorf("corona: invalid image size %d", info.Size())
	}
	return uint64(info.Size()), nil
}

func targetCapacity(f *os.File) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat target: %w", err)
	}
	if isBlockDevice(info.Mode()) {
		var size uint64
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(unix.BLKGETSIZE64), uintptr(unsafe.Pointer(&size)))
		if errno != 0 {
			return 0, fmt.Errorf("get block device size: %w", errno)
		}
		return size, nil
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("corona: unsupported target type %s", info.Mode().Type())
	}
	return uint64(info.Size()), nil
}

func isBlockDevice(mode os.FileMode) bool {
	return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0
}
