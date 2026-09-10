package appliance

import (
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/telemetryos/starforge/platform"
	"github.com/telemetryos/starforge/worker"
)

func buildWorker(ctx context.Context, targetPlatform platform.Spec) (string, func(), error) {
	hostArchitecture, err := platform.HostArchitecture()
	if err != nil {
		return "", func() {}, err
	}
	sourceDirectory, err := sourceRoot()
	if err == nil {
		return buildWorkerFromSource(ctx, sourceDirectory, targetPlatform)
	}
	if os.Getenv("STARFORGE_SOURCE") != "" {
		return "", func() {}, err
	}

	executable, executableErr := os.Executable()
	if executableErr != nil {
		return "", func() {}, executableErr
	}
	for _, candidate := range installedWorkerCandidates(executable, targetPlatform) {
		if _, statErr := os.Stat(candidate); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return "", func() {}, fmt.Errorf("inspecting packaged worker %s: %w", candidate, statErr)
		}
		if validationErr := validateWorker(candidate, targetPlatform); validationErr != nil {
			return "", func() {}, validationErr
		}
		return candidate, func() {}, nil
	}
	if hostArchitecture == targetPlatform.Architecture {
		if validationErr := validateWorker(executable, targetPlatform); validationErr != nil {
			return "", func() {}, validationErr
		}
		return executable, func() {}, nil
	}
	return "", func() {}, fmt.Errorf(
		"no %s appliance worker is installed: install starforge-worker-linux-%s beside the StarForge executable or set STARFORGE_SOURCE to the matching source checkout",
		targetPlatform.Architecture,
		targetPlatform.GoArch(),
	)
}

func buildWorkerFromSource(ctx context.Context, sourceDirectory string, targetPlatform platform.Spec) (string, func(), error) {
	temporaryDir, err := os.MkdirTemp("", "starforge-worker-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(temporaryDir) }
	destination := filepath.Join(temporaryDir, "starforge")
	command := exec.CommandContext(ctx, "go", "build", "-o", destination, "./cmd/starforge")
	command.Dir = sourceDirectory
	command.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+targetPlatform.GoArch(),
	)
	if output, err := command.CombinedOutput(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("building %s worker: %s: %w", targetPlatform.Architecture, strings.TrimSpace(string(output)), err)
	}
	return destination, cleanup, nil
}

func installedWorkerCandidates(executable string, targetPlatform platform.Spec) []string {
	name := "starforge-worker-linux-" + targetPlatform.GoArch()
	if override := os.Getenv("STARFORGE_WORKER_" + strings.ToUpper(targetPlatform.GoArch())); override != "" {
		return []string{override}
	}
	directory := filepath.Dir(executable)
	return []string{
		filepath.Join(directory, name),
		filepath.Join(directory, "starforge-workers", name),
		filepath.Join(filepath.Dir(directory), "lib", "starforge", name),
	}
}

func validateWorker(path string, targetPlatform platform.Spec) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("inspecting appliance worker %s: %w", path, err)
	}
	defer file.Close()
	wantMachine := elf.EM_X86_64
	if targetPlatform.Resolve().Architecture == platform.ArchitectureAArch64 {
		wantMachine = elf.EM_AARCH64
	}
	if file.Machine != wantMachine {
		return fmt.Errorf("appliance worker %s is for %s, not %s", path, file.Machine, targetPlatform.Architecture)
	}
	libraries, err := file.ImportedLibraries()
	if err != nil {
		return fmt.Errorf("inspecting appliance worker linkage: %w", err)
	}
	if len(libraries) != 0 {
		return fmt.Errorf("appliance worker %s is dynamically linked; workers must be built with CGO_ENABLED=0", path)
	}
	return nil
}

func sourceRoot() (string, error) {
	if override := os.Getenv("STARFORGE_SOURCE"); override != "" {
		if _, err := os.Stat(filepath.Join(override, "go.mod")); err != nil {
			return "", fmt.Errorf("STARFORGE_SOURCE does not contain go.mod: %w", err)
		}
		return override, nil
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		candidate := filepath.Dir(filepath.Dir(file))
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cross-architecture worker source is unavailable; set STARFORGE_SOURCE to this StarForge checkout")
}

func createInitramfs(destination, workerPath string, job worker.Job) error {
	workerData, err := os.ReadFile(workerPath)
	if err != nil {
		return fmt.Errorf("reading worker binary: %w", err)
	}
	jobData, err := json.Marshal(job)
	if err != nil {
		return err
	}
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	archive := &newcWriter{writer: gzipWriter, now: uint32(time.Now().Unix())}
	for _, directory := range []string{"dev", "proc", "sys", "run", "newroot"} {
		if err := archive.add(directory, 0o040755, nil); err != nil {
			return closeInitramfs(file, gzipWriter, err)
		}
	}
	if err := archive.add("init", 0o100755, workerData); err != nil {
		return closeInitramfs(file, gzipWriter, err)
	}
	if err := archive.add("starforge-job.json", 0o100644, jobData); err != nil {
		return closeInitramfs(file, gzipWriter, err)
	}
	if err := archive.finish(); err != nil {
		return closeInitramfs(file, gzipWriter, err)
	}
	return closeInitramfs(file, gzipWriter, nil)
}

func closeInitramfs(file *os.File, gzipWriter *gzip.Writer, prior error) error {
	gzipErr := gzipWriter.Close()
	fileErr := file.Close()
	if prior != nil {
		_ = os.Remove(file.Name())
		return prior
	}
	if gzipErr != nil {
		_ = os.Remove(file.Name())
		return gzipErr
	}
	if fileErr != nil {
		_ = os.Remove(file.Name())
		return fileErr
	}
	return nil
}

type newcWriter struct {
	writer io.Writer
	inode  uint32
	now    uint32
}

func (w *newcWriter) add(name string, mode uint32, data []byte) error {
	w.inode++
	header := "070701" +
		hex8(w.inode) + hex8(mode) + hex8(0) + hex8(0) + hex8(1) +
		hex8(w.now) + hex8(uint32(len(data))) + hex8(0) + hex8(0) +
		hex8(0) + hex8(0) + hex8(uint32(len(name)+1)) + hex8(0)
	if _, err := io.WriteString(w.writer, header); err != nil {
		return err
	}
	if _, err := io.WriteString(w.writer, name+"\x00"); err != nil {
		return err
	}
	if err := writePadding(w.writer, 110+len(name)+1); err != nil {
		return err
	}
	if _, err := w.writer.Write(data); err != nil {
		return err
	}
	return writePadding(w.writer, len(data))
}

func (w *newcWriter) finish() error {
	return w.add("TRAILER!!!", 0, nil)
}

func hex8(value uint32) string {
	text := strconv.FormatUint(uint64(value), 16)
	return strings.Repeat("0", 8-len(text)) + text
}

func writePadding(writer io.Writer, length int) error {
	padding := (4 - length%4) % 4
	if padding == 0 {
		return nil
	}
	_, err := writer.Write(make([]byte, padding))
	return err
}
