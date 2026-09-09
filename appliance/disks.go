package appliance

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const minimumProjectDiskSize = int64(512 << 20)

func ensureExt4Disk(path string, size int64, label, source string) error {
	if info, err := os.Stat(path); err == nil && info.Size() == size {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating ext4 disk %s: %w", path, err)
	}
	if err := file.Truncate(size); err != nil {
		file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sizing ext4 disk %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	args := []string{"-q", "-F", "-L", label}
	if source != "" {
		args = append(args, "-d", source)
	}
	args = append(args, path)
	command := exec.Command("mkfs.ext4", args...)
	if output, err := command.CombinedOutput(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("formatting %s: %s: %w", path, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func createProjectDisk(projectDir, destination string) error {
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return err
	}
	tarPath := destination + ".tar"
	size, err := writeProjectTar(projectDir, tarPath)
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)
	diskSize := minimumProjectDiskSize
	if required := alignUp(size*2+(128<<20), 1<<20); required > diskSize {
		diskSize = required
	}
	return ensureExt4Disk(destination, diskSize, "STARFORGE_PROJECT", tarPath)
}

func writeProjectTar(projectDir, destination string) (int64, error) {
	output, err := os.Create(destination)
	if err != nil {
		return 0, err
	}
	writer := tar.NewWriter(output)
	var contentSize int64
	walkErr := filepath.Walk(projectDir, func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(projectDir, filePath)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		first := strings.SplitN(filepath.ToSlash(relative), "/", 2)[0]
		if info.IsDir() && (first == ".git" || first == ".starforge") {
			return filepath.SkipDir
		}
		if first == ".git" || first == ".starforge" {
			return nil
		}
		if info.Mode()&(os.ModeSocket|os.ModeNamedPipe|os.ModeDevice) != 0 {
			return nil
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(filePath)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Uid = 0
		header.Gid = 0
		header.Uname = "root"
		header.Gname = "root"
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		contentSize += info.Size()
		return nil
	})
	closeWriterErr := writer.Close()
	closeFileErr := output.Close()
	if walkErr != nil {
		_ = os.Remove(destination)
		return 0, walkErr
	}
	if closeWriterErr != nil {
		return 0, closeWriterErr
	}
	if closeFileErr != nil {
		return 0, closeFileErr
	}
	return contentSize, nil
}

func alignUp(value, alignment int64) int64 {
	return ((value + alignment - 1) / alignment) * alignment
}
