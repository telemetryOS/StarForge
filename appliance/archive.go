package appliance

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/telemetryos/starforge/platform"
)

type archiveReader struct {
	reader io.Reader
	close  func() error
}

func (m *Manager) ensureAppliance(ctx context.Context, targetPlatform platform.Spec) (*preparedAppliance, error) {
	cacheDir := filepath.Join(m.cacheRoot, string(targetPlatform.Architecture))
	rootDisk := filepath.Join(cacheDir, "root.raw")
	kernel := filepath.Join(cacheDir, "kernel")
	if applianceReady(rootDisk, kernel) {
		return &preparedAppliance{RootDisk: rootDisk, Kernel: kernel}, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating appliance cache: %w", err)
	}
	lock, err := lockState(cacheDir)
	if err != nil {
		return nil, err
	}
	defer unlockState(lock)
	if applianceReady(rootDisk, kernel) {
		return &preparedAppliance{RootDisk: rootDisk, Kernel: kernel}, nil
	}
	archives, err := m.sources(ctx, targetPlatform, cacheDir)
	if err != nil {
		return nil, err
	}
	normalized := filepath.Join(cacheDir, "rootfs.tar.creating")
	if err := normalizeArchives(archives, normalized, kernel, targetPlatform.Architecture); err != nil {
		return nil, err
	}
	defer os.Remove(normalized)
	creating := rootDisk + ".creating"
	if err := os.Remove(creating); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := ensureExt4Disk(creating, 8<<30, "STARFORGE_BUILDER", normalized); err != nil {
		return nil, err
	}
	if err := os.Rename(creating, rootDisk); err != nil {
		return nil, fmt.Errorf("publishing appliance root: %w", err)
	}
	return &preparedAppliance{RootDisk: rootDisk, Kernel: kernel}, nil
}

func applianceReady(rootDisk, kernel string) bool {
	rootInfo, rootErr := os.Stat(rootDisk)
	kernelInfo, kernelErr := os.Stat(kernel)
	return rootErr == nil && rootInfo.Size() > 0 && kernelErr == nil && kernelInfo.Size() > 0
}

type preparedAppliance struct {
	RootDisk string
	Kernel   string
}

func normalizeArchives(archives []sourceArchive, destination, kernelPath string, architecture platform.Architecture) error {
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	writer := tar.NewWriter(output)
	kernelFound := false
	for _, source := range archives {
		if err := appendArchive(writer, source, kernelPath, architecture, &kernelFound); err != nil {
			writer.Close()
			output.Close()
			_ = os.Remove(destination)
			return err
		}
	}
	if err := writer.Close(); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if !kernelFound {
		return fmt.Errorf("appliance archive did not contain a kernel for %s", architecture)
	}
	return nil
}

func appendArchive(writer *tar.Writer, source sourceArchive, kernelPath string, architecture platform.Architecture, kernelFound *bool) error {
	archive, err := openArchive(source.Path)
	if err != nil {
		return err
	}
	defer archive.close()
	reader := tar.NewReader(archive.reader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", source.Path, err)
		}
		name, include, err := normalizedArchivePath(header.Name, source)
		if err != nil {
			return err
		}
		if !include {
			continue
		}
		header.Name = name
		if header.Typeflag == tar.TypeLink && header.Linkname != "" {
			linkName, linkInclude, linkErr := normalizedArchivePath(header.Linkname, sourceArchive{StripPrefix: source.StripPrefix})
			if linkErr != nil {
				return linkErr
			}
			if linkInclude {
				header.Linkname = linkName
			} else {
				return fmt.Errorf("hard link %q points outside the normalized archive", header.Name)
			}
		}
		if err := writer.WriteHeader(header); err != nil {
			return fmt.Errorf("writing normalized header %s: %w", name, err)
		}
		var body io.Writer = writer
		var kernel *os.File
		if !*kernelFound && isKernelPath(name, architecture) {
			kernel, err = os.Create(kernelPath + ".creating")
			if err != nil {
				return err
			}
			body = io.MultiWriter(writer, kernel)
		}
		if _, err := io.Copy(body, reader); err != nil {
			if kernel != nil {
				kernel.Close()
			}
			return fmt.Errorf("copying normalized entry %s: %w", name, err)
		}
		if kernel != nil {
			if err := kernel.Close(); err != nil {
				return err
			}
			if err := os.Rename(kernelPath+".creating", kernelPath); err != nil {
				return err
			}
			*kernelFound = true
		}
	}
}

func normalizedArchivePath(raw string, source sourceArchive) (string, bool, error) {
	name := strings.TrimPrefix(filepath.ToSlash(raw), "./")
	if source.StripPrefix != "" {
		if name == strings.TrimSuffix(source.StripPrefix, "/") {
			return "", false, nil
		}
		if !strings.HasPrefix(name, source.StripPrefix) {
			return "", false, nil
		}
		name = strings.TrimPrefix(name, source.StripPrefix)
	}
	name = strings.TrimPrefix(name, "/")
	clean := path.Clean(name)
	if clean == "." || clean == "" {
		return "", false, nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("archive path %q escapes its root", raw)
	}
	if source.Metadata && strings.HasPrefix(clean, ".") && !strings.Contains(clean, "/") {
		return "", false, nil
	}
	return clean, true, nil
}

func isKernelPath(name string, architecture platform.Architecture) bool {
	if architecture == platform.ArchitectureAArch64 {
		return name == "boot/Image"
	}
	return strings.HasPrefix(name, "usr/lib/modules/") && strings.HasSuffix(name, "/vmlinuz")
}

func openArchive(path string) (*archiveReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		reader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, err
		}
		return &archiveReader{reader: reader, close: func() error {
			readerErr := reader.Close()
			fileErr := file.Close()
			if readerErr != nil {
				return readerErr
			}
			return fileErr
		}}, nil
	}
	if strings.HasSuffix(path, ".zst") {
		reader, err := zstd.NewReader(file)
		if err != nil {
			file.Close()
			return nil, err
		}
		return &archiveReader{reader: reader, close: func() error {
			reader.Close()
			return file.Close()
		}}, nil
	}
	return &archiveReader{reader: file, close: file.Close}, nil
}

func extractArchiveEntry(archivePath, wanted, destination string) error {
	archive, err := openArchive(archivePath)
	if err != nil {
		return err
	}
	defer archive.close()
	reader := tar.NewReader(archive.reader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return fmt.Errorf("entry %q not found in %s", wanted, archivePath)
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(filepath.ToSlash(header.Name), "./")
		if name != wanted {
			continue
		}
		file, err := os.Create(destination + ".creating")
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := file.Close()
		if copyErr != nil {
			_ = os.Remove(destination + ".creating")
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Rename(destination+".creating", destination)
	}
}
