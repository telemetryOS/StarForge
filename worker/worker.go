// Package worker runs the privileged StarForge engine inside a build appliance.
package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/platform"
)

const (
	jobPath      = "/starforge-job.json"
	resultPrefix = "STARFORGE_RESULT "
	newRoot      = "/newroot"
	projectDir   = "/project"
	buildRoot    = "/work/build"
	outputDevice = "/dev/vdd"
)

// Job is the immutable build request embedded in the worker initramfs.
type Job struct {
	Target   string        `json:"target"`
	Platform platform.Spec `json:"platform"`
	Clean    bool          `json:"clean,omitempty"`
}

// Result is the structured completion record emitted over the serial console.
type Result struct {
	Success    bool                `json:"success"`
	Error      string              `json:"error,omitempty"`
	SourceHash string              `json:"sourceHash,omitempty"`
	Build      *engine.BuildResult `json:"build,omitempty"`
}

// RunInit mounts appliance resources, executes one job, and powers off the VM.
func RunInit() error {
	result, runErr := run()
	if runErr != nil {
		result = &Result{Success: false, Error: runErr.Error()}
	}
	emitResult(result)
	unix.Sync()
	if rebootErr := unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF); rebootErr != nil {
		if runErr != nil {
			return fmt.Errorf("%w; powering off: %v", runErr, rebootErr)
		}
		return fmt.Errorf("powering off: %w", rebootErr)
	}
	return runErr
}

func run() (*Result, error) {
	job, err := loadJob(jobPath)
	if err != nil {
		return nil, err
	}
	if err := mountAppliance(); err != nil {
		return nil, err
	}
	if err := prepareKernelDevices(); err != nil {
		return nil, err
	}
	if err := configureNetwork(); err != nil {
		return nil, err
	}
	if err := configureAppliancePacman(); err != nil {
		return nil, err
	}
	if err := ensureBuildTools(); err != nil {
		return nil, err
	}

	project, err := config.LoadProject(projectDir)
	if err != nil {
		return nil, err
	}
	project.BuildRoot = buildRoot
	target, ok := project.Targets[job.Target]
	if !ok {
		return nil, fmt.Errorf("target %q not found", job.Target)
	}
	if target.ResolvedPlatform() != job.Platform.Resolve() {
		return nil, fmt.Errorf("target platform %s does not match job platform %s", target.ResolvedPlatform().Identity(), job.Platform.Resolve().Identity())
	}

	targetBuildDir := project.TargetBuildDir(job.Target)
	if err := os.MkdirAll(targetBuildDir, 0o755); err != nil {
		return nil, err
	}
	output, err := engine.InitOutput(targetBuildDir, "worker", job.Target)
	if err != nil {
		return nil, err
	}
	defer output.Close()

	builder := engine.NewBuilderWithHostTools(project, engine.SystemHostTools{})
	builder.SetRuntimeBinary("/usr/lib/starforge/worker")
	var buildContext *actions.BuildContext
	err = output.Run(func() error {
		var buildErr error
		if job.Clean {
			if buildErr = builder.Build(job.Target, target, true); buildErr == nil {
				buildContext, buildErr = builder.EnsurePackaged(job.Target)
			}
		} else {
			buildContext, buildErr = builder.EnsureBuiltAndPackaged(job.Target)
		}
		if buildErr != nil {
			return buildErr
		}
		if buildErr = engine.WriteToDevice(buildContext.Partitions, outputDevice, targetBuildDir); buildErr != nil {
			return buildErr
		}
		if engine.HasInstallerActions(buildContext) {
			buildErr = builder.BundleInstallerToDevice(buildContext, outputDevice)
		}
		return buildErr
	})
	if err != nil {
		return nil, err
	}

	buildResult, err := engine.LoadBuildResult(targetBuildDir)
	if err != nil {
		return nil, err
	}
	buildResult.Partitions = buildContext.Partitions
	manifest, err := engine.LoadManifest(filepath.Join(targetBuildDir, "cache"))
	if err != nil {
		return nil, err
	}
	if manifest.Packaging == nil || !manifest.Packaging.Completed {
		return nil, fmt.Errorf("worker packaging did not complete")
	}
	return &Result{Success: true, SourceHash: manifest.Packaging.Hash, Build: buildResult}, nil
}

func configureAppliancePacman() error {
	server := "Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch\n"
	if runtime.GOARCH == "arm64" {
		server = "Server = https://ca.us.mirror.archlinuxarm.org/$arch/$repo\n"
	}
	path := "/etc/pacman.d/mirrorlist"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(server), 0o644); err != nil {
		return fmt.Errorf("configuring appliance mirror: %w", err)
	}
	return nil
}

func loadJob(path string) (*Job, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading worker job: %w", err)
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("parsing worker job: %w", err)
	}
	if job.Target == "" {
		return nil, fmt.Errorf("worker target is required")
	}
	if err := job.Platform.Validate(); err != nil {
		return nil, fmt.Errorf("worker platform: %w", err)
	}
	return &job, nil
}

func emitResult(result *Result) {
	data, err := json.Marshal(result)
	if err != nil {
		fmt.Printf("%s{\"success\":false,\"error\":%q}\n", resultPrefix, err.Error())
		return
	}
	fmt.Printf("%s%s\n", resultPrefix, data)
}

func mountAppliance() error {
	for _, dir := range []string{"/dev", "/dev/pts", "/proc", "/sys", "/run", newRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, "mode=0755"); err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("mounting devtmpfs: %w", err)
	}
	if err := os.MkdirAll("/dev/pts", 0o755); err != nil {
		return err
	}
	if err := unix.Mount("devpts", "/dev/pts", "devpts", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("mounting devpts: %w", err)
	}
	if err := os.MkdirAll("/dev/shm", 0o1777); err != nil {
		return err
	}
	if err := unix.Mount("tmpfs", "/dev/shm", "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "mode=1777"); err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("mounting shared memory: %w", err)
	}
	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("mounting proc: %w", err)
	}
	for name, target := range map[string]string{
		"fd": "/proc/self/fd", "stdin": "/proc/self/fd/0", "stdout": "/proc/self/fd/1", "stderr": "/proc/self/fd/2",
	} {
		path := filepath.Join("/dev", name)
		if err := os.Symlink(target, path); err != nil && !os.IsExist(err) {
			return fmt.Errorf("creating %s: %w", path, err)
		}
	}
	if err := unix.Mount("sysfs", "/sys", "sysfs", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("mounting sysfs: %w", err)
	}
	if err := unix.Mount("tmpfs", "/run", "tmpfs", 0, "mode=0755"); err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("mounting run tmpfs: %w", err)
	}

	for _, device := range []string{"/dev/vda", "/dev/vdb", "/dev/vdc", outputDevice} {
		if err := waitForPath(device, 30*time.Second); err != nil {
			return err
		}
	}
	if err := unix.Mount("/dev/vda", newRoot, "ext4", 0, ""); err != nil {
		return fmt.Errorf("mounting appliance root: %w", err)
	}
	for _, dir := range []string{"dev", "proc", "sys", "run", "project", "work", "usr/lib/starforge"} {
		if err := os.MkdirAll(filepath.Join(newRoot, dir), 0o755); err != nil {
			return err
		}
	}
	if err := copyFile("/proc/self/exe", filepath.Join(newRoot, "usr/lib/starforge/worker"), 0o755); err != nil {
		return fmt.Errorf("staging worker binary: %w", err)
	}
	if err := unix.Mount("/dev/vdb", filepath.Join(newRoot, "project"), "ext4", unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("mounting project: %w", err)
	}
	if err := unix.Mount("/dev/vdc", filepath.Join(newRoot, "work"), "ext4", 0, ""); err != nil {
		return fmt.Errorf("mounting work disk: %w", err)
	}
	for _, mountPoint := range []string{"dev", "proc", "sys", "run"} {
		if err := unix.Mount("/"+mountPoint, filepath.Join(newRoot, mountPoint), "", unix.MS_MOVE, ""); err != nil {
			return fmt.Errorf("moving %s mount: %w", mountPoint, err)
		}
	}
	if err := unix.Chroot(newRoot); err != nil {
		return fmt.Errorf("entering appliance root: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	for _, dir := range []string{"/work/build", "/work/state", "/work/tmp", "/tmp"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	os.Setenv("HOME", "/root")
	os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/bin")
	os.Setenv("XDG_STATE_HOME", "/work/state")
	os.Setenv("STARFORGE_TMPDIR", "/work/tmp")
	os.Setenv("STARFORGE_NONINTERACTIVE", "1")
	os.Setenv("TMPDIR", "/work/tmp")
	return nil
}

func configureNetwork() error {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return err
	}
	interfaceName := ""
	for _, entry := range entries {
		if entry.Name() != "lo" {
			interfaceName = entry.Name()
			break
		}
	}
	if interfaceName == "" {
		return fmt.Errorf("no network interface found")
	}
	commands := [][]string{
		{"link", "set", "lo", "up"},
		{"link", "set", interfaceName, "up"},
		{"addr", "add", "10.0.2.15/24", "dev", interfaceName},
		{"route", "add", "default", "via", "10.0.2.2"},
	}
	for _, args := range commands {
		output, commandErr := exec.Command("ip", args...).CombinedOutput()
		if commandErr != nil && !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("ip %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), commandErr)
		}
	}
	if err := os.Remove("/etc/resolv.conf"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile("/etc/resolv.conf", []byte("nameserver 10.0.2.3\n"), 0o644)
}

func prepareKernelDevices() error {
	if output, err := exec.Command("depmod", "-a").CombinedOutput(); err != nil {
		return fmt.Errorf("indexing kernel modules: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if entries, err := os.ReadDir("/sys/class/net"); err != nil {
		return err
	} else if len(entries) == 1 && entries[0].Name() == "lo" {
		if output, err := exec.Command("modprobe", "virtio_net").CombinedOutput(); err != nil {
			return fmt.Errorf("loading virtio_net: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}
	if output, err := exec.Command("modprobe", "loop").CombinedOutput(); err != nil {
		if _, statErr := os.Stat("/dev/loop-control"); statErr != nil {
			return fmt.Errorf("loading loop module: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}
	if err := makeDevice("/dev/loop-control", unix.S_IFCHR|0o600, 10, 237); err != nil {
		return err
	}
	for index := 0; index < 64; index++ {
		if err := makeDevice(fmt.Sprintf("/dev/loop%d", index), unix.S_IFBLK|0o600, 7, uint32(index)); err != nil {
			return err
		}
	}
	return nil
}

func makeDevice(path string, mode uint32, major, minor uint32) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := unix.Mknod(path, mode, int(unix.Mkdev(major, minor))); err != nil && !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("creating device %s: %w", path, err)
	}
	return nil
}

func ensureBuildTools() error {
	tools := engine.SystemHostTools{}
	if err := tools.Ensure("build"); err == nil {
		return nil
	}
	if err := initializePackageKeyring(); err != nil {
		return err
	}
	packages := []string{
		"arch-install-scripts", "dosfstools", "e2fsprogs", "gnupg", "gptfdisk",
		"parted", "tar", "util-linux", "zstd",
	}
	cacheDir := "/work/appliance-pacman"
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating appliance package cache: %w", err)
	}
	args := append([]string{"-Sy", "--needed", "--noconfirm", "--cachedir", cacheDir}, packages...)
	command := exec.Command("pacman", args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("installing appliance build tools: %w", err)
	}
	return tools.Ensure("build")
}

func initializePackageKeyring() error {
	if _, err := os.Stat("/etc/pacman.d/gnupg/pubring.gpg"); err == nil {
		return nil
	}
	commands := [][]string{{"--init"}, {"--populate", "archlinux"}}
	if runtime.GOARCH == "arm64" {
		commands[1] = []string{"--populate", "archlinuxarm"}
	}
	for _, args := range commands {
		command := exec.Command("pacman-key", args...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("pacman-key %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

func waitForPath(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

func copyFile(source, destination string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, mode)
}
