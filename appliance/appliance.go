// Package appliance manages rootless QEMU build appliances.
package appliance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/platform"
	"github.com/telemetryos/starforge/worker"
)

const applianceVersion = "1"

const workDiskSize = int64(128 << 30)

// Request contains one isolated builder-VM invocation.
type Request struct {
	Project    *config.Project
	TargetName string
	Platform   platform.Spec
	Clean      bool
	OutputDisk string
}

// Manager prepares and launches architecture-specific build appliances.
type Manager struct {
	cacheRoot string
}

// NewManager creates an appliance manager using the user's persistent cache.
func NewManager() (*Manager, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolving appliance cache: %w", err)
	}
	return &Manager{cacheRoot: filepath.Join(cacheDir, "starforge", "appliances", applianceVersion)}, nil
}

// Prepare validates the rootless host tools needed for a target architecture.
func (m *Manager) Prepare(targetPlatform platform.Spec) error {
	targetPlatform = targetPlatform.Resolve()
	if err := targetPlatform.Validate(); err != nil {
		return err
	}
	for _, tool := range []string{"gpg", "mkfs.ext4", "qemu-img", qemuBinary(targetPlatform)} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is required by the VM executor: %w", tool, err)
		}
	}
	return nil
}

// Run constructs per-build inputs, boots the worker, and returns its result.
func (m *Manager) Run(ctx context.Context, request Request) (*worker.Result, error) {
	if request.Project == nil {
		return nil, fmt.Errorf("project is required")
	}
	if request.OutputDisk == "" {
		return nil, fmt.Errorf("output disk is required")
	}
	targetPlatform := request.Platform.Resolve()
	if err := m.Prepare(targetPlatform); err != nil {
		return nil, err
	}

	stateDir := filepath.Join(
		request.Project.BuildDir(), "vm", string(targetPlatform.Architecture),
		string(targetPlatform.Distribution),
	)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating VM state directory: %w", err)
	}
	lock, err := lockState(stateDir)
	if err != nil {
		return nil, err
	}
	defer unlockState(lock)

	prepared, err := m.ensureAppliance(ctx, targetPlatform)
	if err != nil {
		return nil, err
	}
	workDisk := filepath.Join(stateDir, "work.raw")
	if err := ensureExt4Disk(workDisk, workDiskSize, "STARFORGE_WORK", ""); err != nil {
		return nil, err
	}
	rootOverlay := filepath.Join(stateDir, "root-overlay.qcow2")
	if err := createRootOverlay(ctx, prepared.RootDisk, rootOverlay); err != nil {
		return nil, err
	}
	defer os.Remove(rootOverlay)

	projectDisk := filepath.Join(stateDir, "project.raw.creating")
	if err := createProjectDisk(request.Project.Dir, projectDisk); err != nil {
		return nil, err
	}
	defer os.Remove(projectDisk)

	workerPath, cleanupWorker, err := buildWorker(ctx, targetPlatform)
	if err != nil {
		return nil, err
	}
	defer cleanupWorker()
	job := worker.Job{
		Protocol:    worker.ProtocolVersion,
		Target:      request.TargetName,
		Platform:    targetPlatform,
		Clean:       request.Clean,
		Environment: request.Project.ReferencedHostEnvironment(),
	}
	initramfs := filepath.Join(stateDir, "worker-initramfs.gz.creating")
	if err := createInitramfs(initramfs, workerPath, job); err != nil {
		return nil, err
	}
	defer os.Remove(initramfs)

	return runQEMU(ctx, launchRequest{
		Platform:    targetPlatform,
		RootDisk:    rootOverlay,
		Kernel:      prepared.Kernel,
		Initramfs:   initramfs,
		ProjectDisk: projectDisk,
		WorkDisk:    workDisk,
		OutputDisk:  request.OutputDisk,
	})
}

func createRootOverlay(ctx context.Context, rootDisk, destination string) error {
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return err
	}
	absoluteRoot, err := filepath.Abs(rootDisk)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "qemu-img", "create", "-q", "-f", "qcow2", "-F", "raw", "-b", absoluteRoot, destination)
	if output, err := command.CombinedOutput(); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("creating ephemeral appliance root: %s: %w", string(output), err)
	}
	return nil
}

func lockState(stateDir string) (*os.File, error) {
	path := filepath.Join(stateDir, "build.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening VM state lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()
		return nil, fmt.Errorf("locking VM state: %w", err)
	}
	return file, nil
}

func unlockState(file *os.File) {
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	_ = file.Close()
}
