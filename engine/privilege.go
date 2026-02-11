package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// EnsureRootExec re-execs the current process under sudo if not already root.
func EnsureRootExec() error {
	if os.Geteuid() == 0 {
		return nil
	}

	sudo, err := exec_LookPath("sudo")
	if err != nil {
		return fmt.Errorf("sudo not found: %w", err)
	}

	args := append([]string{sudo}, os.Args...)
	return syscall.Exec(sudo, args, os.Environ())
}

// ChownToInvoker recursively changes ownership of paths back to the user who
// invoked sudo. This is a no-op when not running under sudo.
func ChownToInvoker(paths ...string) {
	uidStr := os.Getenv("SUDO_UID")
	gidStr := os.Getenv("SUDO_GID")
	if uidStr == "" || gidStr == "" {
		return
	}

	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		return
	}

	for _, p := range paths {
		filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			os.Lchown(path, uid, gid)
			return nil
		})
	}
}

// exec_LookPath is a thin wrapper so we don't pull in os/exec just for LookPath.
func exec_LookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		path := filepath.Join(dir, name)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s: executable file not found in $PATH", name)
}
