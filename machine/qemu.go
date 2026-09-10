package machine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/platform"
)

// QEMU boots sealed raw disk artifacts with architecture-specific QEMU machines.
type QEMU struct{}

// LaunchSpec is an executable and argument vector for a QEMU target VM.
type LaunchSpec struct {
	Binary string
	Args   []string
}

type qemuHost struct {
	lookPath     func(string) (string, error)
	findFirmware func(platform.Spec) (string, error)
	kvmAvailable func(platform.Spec) bool
}

// Run launches QEMU and attaches its interactive streams to the coordinator.
func (*QEMU) Run(ctx context.Context, request RunRequest) error {
	spec, err := BuildLaunchSpec(request)
	if err != nil {
		return err
	}
	if err := ensureAdditionalDisks(request.BuildDir, request.Config); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, spec.Binary, spec.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", spec.Binary, err)
	}
	go func() {
		select {
		case signalValue := <-signals:
			_ = cmd.Process.Signal(signalValue)
		case <-ctx.Done():
		}
	}()
	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return fmt.Errorf("qemu: %w", err)
	}
	return nil
}

// BuildLaunchSpec resolves the architecture-specific QEMU command line.
func BuildLaunchSpec(request RunRequest) (*LaunchSpec, error) {
	return buildLaunchSpec(request, qemuHost{
		lookPath:     exec.LookPath,
		findFirmware: findFirmware,
		kvmAvailable: kvmAvailable,
	})
}

func buildLaunchSpec(request RunRequest, host qemuHost) (*LaunchSpec, error) {
	if request.Artifact == nil {
		return nil, fmt.Errorf("artifact receipt is required")
	}
	if request.Artifact.State != artifact.StateSealed {
		return nil, fmt.Errorf("artifact is not sealed")
	}
	if _, err := os.Stat(request.Disk); err != nil {
		return nil, fmt.Errorf("opening artifact disk: %w", err)
	}
	targetPlatform := request.Artifact.Platform.Resolve()
	if err := targetPlatform.Validate(); err != nil {
		return nil, fmt.Errorf("artifact platform: %w", err)
	}

	binary := "qemu-system-x86_64"
	if targetPlatform.Architecture == platform.ArchitectureAArch64 {
		binary = "qemu-system-aarch64"
	}
	if _, err := host.lookPath(binary); err != nil {
		return nil, fmt.Errorf("%s is required to run %s targets: %w", binary, targetPlatform.Architecture, err)
	}

	memory, cpus, sshPort := 4096, 4, 2222
	display := "gtk,gl=on,show-cursor=off,zoom-to-fit=on"
	cpu := ""
	if request.Config != nil {
		if request.Config.Memory > 0 {
			memory = request.Config.Memory
		}
		if request.Config.CPUs > 0 {
			cpus = request.Config.CPUs
		}
		if request.Config.SSHPort > 0 {
			sshPort = request.Config.SSHPort
		}
		if request.Config.Display != "" {
			display = request.Config.Display
		}
		cpu = request.Config.CPU
	}

	kvm := host.kvmAvailable(targetPlatform)
	if cpu == "" {
		if kvm {
			cpu = "host"
		} else {
			cpu = "max"
		}
	}
	args := []string{
		"-m", fmt.Sprintf("%dM", memory),
		"-smp", strconv.Itoa(cpus),
		"-cpu", cpu,
	}
	if kvm {
		args = append(args, "-enable-kvm")
	} else {
		args = append(args, "-accel", "tcg")
	}

	firmware, err := host.findFirmware(targetPlatform)
	if err != nil {
		return nil, err
	}
	if targetPlatform.Architecture == platform.ArchitectureAArch64 {
		args = append(args, "-machine", "virt", "-bios", firmware)
	} else {
		args = append(args, "-machine", "q35", "-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", firmware))
	}

	if request.Serial {
		args = append(args, "-display", "none", "-serial", "mon:stdio")
	} else {
		gpuMemory := 512
		if request.Config != nil && request.Config.GPUMemory > 0 {
			gpuMemory = request.Config.GPUMemory
		}
		gpu := fmt.Sprintf("virtio-vga-gl,max_hostmem=%dM,blob=on,edid=on,xres=1920,yres=1080", gpuMemory)
		if targetPlatform.Architecture == platform.ArchitectureAArch64 {
			gpu = "virtio-gpu-pci"
		}
		args = append(args, "-device", gpu, "-display", display)
		if targetPlatform.Architecture == platform.ArchitectureAArch64 {
			args = append(args, "-device", "qemu-xhci,id=xhci", "-device", "usb-tablet,bus=xhci.0")
		} else {
			args = append(args, "-usb", "-device", "usb-tablet")
		}
	}

	args = append(args,
		"-device", "virtio-rng-pci",
		"-device", "virtio-balloon-pci",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", sshPort),
		"-device", "virtio-net-pci,netdev=net0",
		"-drive", fmt.Sprintf("file=%s,format=raw,if=virtio,snapshot=on", request.Disk),
	)

	if request.Config != nil {
		for _, disk := range request.Config.Disks {
			path := filepath.Join(request.BuildDir, "disks", disk.Name+".img")
			args = append(args, "-drive", fmt.Sprintf("file=%s,format=raw,if=virtio,cache=writeback,aio=threads", path))
		}
		args = append(args, request.Config.Args...)
	}
	return &LaunchSpec{Binary: binary, Args: args}, nil
}

func kvmAvailable(targetPlatform platform.Spec) bool {
	hostArchitecture := platform.ArchitectureX86_64
	if runtime.GOARCH == "arm64" {
		hostArchitecture = platform.ArchitectureAArch64
	}
	if hostArchitecture != targetPlatform.Architecture {
		return false
	}
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

func findFirmware(targetPlatform platform.Spec) (string, error) {
	var candidates []string
	if targetPlatform.Architecture == platform.ArchitectureAArch64 {
		candidates = []string{
			"/usr/share/edk2/aarch64/QEMU_EFI.fd",
			"/usr/share/edk2/aarch64/QEMU_EFI-pflash.raw",
			"/usr/share/AAVMF/AAVMF_CODE.fd",
			"/usr/share/qemu-efi-aarch64/QEMU_EFI.fd",
		}
	} else {
		candidates = []string{
			"/usr/share/edk2/x64/OVMF_CODE.4m.fd",
			"/usr/share/edk2/x64/OVMF_CODE.fd",
			"/usr/share/OVMF/OVMF_CODE.fd",
			"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd",
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("UEFI firmware not found for %s", targetPlatform.Architecture)
}

func ensureAdditionalDisks(buildDir string, qemuConfig *config.QEMUConfig) error {
	if qemuConfig == nil || len(qemuConfig.Disks) == 0 {
		return nil
	}
	diskDir := filepath.Join(buildDir, "disks")
	if err := os.MkdirAll(diskDir, 0o755); err != nil {
		return fmt.Errorf("creating disk directory: %w", err)
	}
	for _, disk := range qemuConfig.Disks {
		path := filepath.Join(diskDir, disk.Name+".img")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		size, _, err := actions.ParseSize(disk.Size)
		if err != nil {
			return fmt.Errorf("disk %q: %w", disk.Name, err)
		}
		file, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("creating disk %q: %w", disk.Name, err)
		}
		if err := file.Truncate(int64(size)); err != nil {
			file.Close()
			return fmt.Errorf("sizing disk %q: %w", disk.Name, err)
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}
