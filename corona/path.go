package corona

import (
	"fmt"
	"os"
	"path/filepath"
)

func rejectSamePath(a, b, label string) error {
	absA, err := filepath.Abs(a)
	if err != nil {
		return fmt.Errorf("resolve %s source path: %w", label, err)
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return fmt.Errorf("resolve %s output path: %w", label, err)
	}
	if filepath.Clean(absA) == filepath.Clean(absB) {
		return fmt.Errorf("corona: %s input and output must be different paths", label)
	}
	infoA, errA := os.Stat(absA)
	infoB, errB := os.Stat(absB)
	if errA == nil && errB == nil && os.SameFile(infoA, infoB) {
		return fmt.Errorf("corona: %s input and output refer to the same file", label)
	}
	return nil
}

func requireBlockDevice(path string, f *os.File, role string) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", role, err)
	}
	if !isBlockDevice(info.Mode()) {
		return fmt.Errorf("corona: %s must be a block device: %s", role, path)
	}
	return nil
}
