package appliance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/telemetryos/starforge/platform"
	"github.com/telemetryos/starforge/worker"
)

type launchRequest struct {
	Platform    platform.Spec
	RootDisk    string
	Kernel      string
	Initramfs   string
	ProjectDisk string
	WorkDisk    string
	OutputDisk  string
}

func runQEMU(ctx context.Context, request launchRequest) (*worker.Result, error) {
	binary, args, err := buildLaunchCommand(request)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, binary, args...)
	capture := &resultCapture{}
	command.Stdin = os.Stdin
	command.Stdout = io.MultiWriter(retryingWriter{ctx: ctx, writer: os.Stdout}, capture)
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if result := capture.result(); result != nil && !result.Success {
			return result, fmt.Errorf("builder VM: %s", result.Error)
		}
		return nil, fmt.Errorf("builder VM exited unexpectedly: %w", err)
	}
	result := capture.result()
	if result == nil {
		return nil, fmt.Errorf("builder VM exited without a completion record")
	}
	if !result.Success {
		return result, fmt.Errorf("builder VM: %s", result.Error)
	}
	if result.Build == nil {
		return nil, fmt.Errorf("builder VM returned no build result")
	}
	return result, nil
}

type retryingWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w retryingWriter) Write(data []byte) (int, error) {
	written := 0
	for written < len(data) {
		n, err := w.writer.Write(data[written:])
		written += n
		if err == nil {
			if n == 0 {
				return written, io.ErrNoProgress
			}
			continue
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return written, err
		}
		select {
		case <-w.ctx.Done():
			return written, w.ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return written, nil
}

func buildLaunchCommand(request launchRequest) (string, []string, error) {
	binary := qemuBinary(request.Platform)
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return "", nil, err
	}
	console := "ttyS0"
	machine := "q35"
	if request.Platform.Architecture == platform.ArchitectureAArch64 {
		console = "ttyAMA0"
		machine = "virt"
	}
	cpu := "max"
	acceleration := []string{"-accel", "tcg,thread=multi"}
	if builderKVMAvailable(request.Platform) {
		cpu = "host"
		acceleration = []string{"-enable-kvm"}
	}
	args := []string{
		"-machine", machine,
		"-cpu", cpu,
		"-m", "8192M",
		"-smp", "4",
	}
	args = append(args, acceleration...)
	args = append(args,
		"-kernel", request.Kernel,
		"-initrd", request.Initramfs,
		"-append", fmt.Sprintf("console=%s loglevel=4 panic=1", console),
		"-display", "none",
		"-serial", "mon:stdio",
		"-no-reboot",
		"-device", "virtio-rng-pci",
		"-netdev", "user,id=net0",
		"-device", "virtio-net-pci,netdev=net0",
	)
	for _, disk := range []struct {
		id       string
		path     string
		format   string
		readOnly bool
	}{
		{id: "root", path: request.RootDisk, format: "qcow2"},
		{id: "project", path: request.ProjectDisk, format: "raw", readOnly: true},
		{id: "work", path: request.WorkDisk, format: "raw"},
		{id: "output", path: request.OutputDisk, format: "raw"},
	} {
		if info, statErr := os.Stat(disk.path); statErr != nil || info.Size() == 0 {
			if statErr == nil {
				statErr = fmt.Errorf("file is empty")
			}
			return "", nil, fmt.Errorf("opening %s disk %s: %w", disk.id, disk.path, statErr)
		}
		drive := fmt.Sprintf("if=none,id=%s,file=%s,format=%s,cache=writeback", disk.id, disk.path, disk.format)
		if disk.readOnly {
			drive += ",readonly=on"
		}
		args = append(args, "-drive", drive, "-device", fmt.Sprintf("virtio-blk-pci,drive=%s", disk.id))
	}
	return resolved, args, nil
}

func qemuBinary(targetPlatform platform.Spec) string {
	if targetPlatform.Resolve().Architecture == platform.ArchitectureAArch64 {
		return "qemu-system-aarch64"
	}
	return "qemu-system-x86_64"
}

func builderKVMAvailable(targetPlatform platform.Spec) bool {
	host := platform.ArchitectureX86_64
	if runtime.GOARCH == "arm64" {
		host = platform.ArchitectureAArch64
	}
	if host != targetPlatform.Resolve().Architecture {
		return false
	}
	file, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	return file.Close() == nil
}

type resultCapture struct {
	mu      sync.Mutex
	pending []byte
	value   *worker.Result
}

func (c *resultCapture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = append(c.pending, data...)
	for {
		index := bytes.IndexByte(c.pending, '\n')
		if index < 0 {
			break
		}
		c.parseLine(string(c.pending[:index]))
		c.pending = c.pending[index+1:]
	}
	if len(c.pending) > 1<<20 {
		c.pending = c.pending[len(c.pending)-(1<<20):]
	}
	return len(data), nil
}

func (c *resultCapture) parseLine(line string) {
	index := strings.Index(line, "STARFORGE_RESULT ")
	if index < 0 {
		return
	}
	var result worker.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(line[index+len("STARFORGE_RESULT "):])), &result); err == nil {
		c.value = &result
	}
}

func (c *resultCapture) result() *worker.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) > 0 {
		c.parseLine(string(c.pending))
	}
	return c.value
}
