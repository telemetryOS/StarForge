package engine

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// parseMode parses an octal mode string, returning defaultMode when s is empty.
// Returns an error if s is non-empty but cannot be parsed as an octal mode.
func parseMode(s string, defaultMode os.FileMode) (os.FileMode, error) {
	if s == "" {
		return defaultMode, nil
	}
	var mode uint32
	if _, err := fmt.Sscanf(s, "%o", &mode); err != nil {
		return 0, fmt.Errorf("invalid file mode %q: %w", s, err)
	}
	return os.FileMode(mode), nil
}

// writeFile writes content to a file, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// appendFile appends content to a file, creating it if it doesn't exist.
func appendFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// CopyFile copies a file from src to dest using streaming I/O.
func CopyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// ChrootRun executes a command inside the rootfs using arch-chroot.
func ChrootRun(rootfs string, args ...string) error {
	cmdArgs := append([]string{rootfs}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	if out != nil {
		w := out.LogWriter()
		cmd.Stdout = w
		cmd.Stderr = w
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// inheritOwnership sets the ownership of path to match its immediate parent
// directory. If path is a directory, ownership is applied recursively to all
// contents. No-op if the parent is root:root or cannot be stat'd.
func inheritOwnership(path string) {
	parent := filepath.Dir(path)
	var st syscall.Stat_t
	if err := syscall.Lstat(parent, &st); err != nil {
		return
	}
	if st.Uid == 0 && st.Gid == 0 {
		return
	}
	uid := int(st.Uid)
	gid := int(st.Gid)

	info, err := os.Lstat(path)
	if err != nil {
		return
	}
	if !info.IsDir() {
		os.Lchown(path, uid, gid)
		return
	}

	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		os.Lchown(p, uid, gid)
		return nil
	})
}

// mkdirAllInherit is like os.MkdirAll but newly created directories inherit
// the ownership of their parent directory. This ensures intermediate dirs
// (e.g. .config/sway/ inside /home/player/) get the correct ownership
// instead of defaulting to root:root.
func mkdirAllInherit(path string, perm os.FileMode) error {
	// Fast path: already exists
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil
	}

	// Collect directories that need to be created (bottom-up)
	var toCreate []string
	dir := filepath.Clean(path)
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		toCreate = append(toCreate, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Create top-down, inheriting ownership at each level
	for i := len(toCreate) - 1; i >= 0; i-- {
		d := toCreate[i]
		if err := os.Mkdir(d, perm); err != nil && !os.IsExist(err) {
			return err
		}
		inheritOwnership(d)
	}
	return nil
}
