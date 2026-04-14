package engine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

// deviceMapperName returns a deterministic device mapper name for a target.
// Format: starforge-<target>-<hash8> where hash8 is the first 8 chars of
// SHA-256 of the absolute project directory.
func deviceMapperName(targetName, projectDir string) string {
	h := sha256.Sum256([]byte(projectDir))
	return fmt.Sprintf("starforge-%s-%.8x", targetName, h[:4])
}

// cleanupDeviceMapper performs idempotent cleanup of a device mapper device
// and its backing loop devices. Errors are logged but never returned.
func cleanupDeviceMapper(dmName string, loopDevs []string) {
	// Sync to flush writes
	syscall.Sync()

	// Wait for udev to finish processing events — it may hold partition
	// sub-devices open briefly after QEMU exits.
	run("udevadm", "settle")

	// Remove kernel partition mappings first (e.g. /dev/mapper/starforge-device-abcd1234p1).
	// These are created by partprobe/kpartx when the GPT is written, and must be
	// removed before the main dm device.
	entries, _ := filepath.Glob(fmt.Sprintf("/dev/mapper/%sp*", dmName))
	for _, entry := range entries {
		run("dmsetup", "remove", "--force", filepath.Base(entry))
	}

	// Remove the main dm device
	dmPath := filepath.Join("/dev/mapper", dmName)
	if _, err := os.Stat(dmPath); err == nil {
		run("dmsetup", "remove", "--force", dmName)
	}

	// Detach loop devices
	for _, loopDev := range loopDevs {
		run("losetup", "-d", loopDev)
	}
}

// partEntry tracks partition layout for GPT creation after dm device is assembled.
type partEntry struct {
	num         int
	startSector uint64
	endSector   uint64 // inclusive
	name        string
	filesystem  string
	partType    string
}

// setupDeviceMapper creates a device mapper device that assembles individual
// partition images into a single virtual block device with a GPT partition table.
//
// Layout: [GPT header 1MB] [part1] [part2] ... [partN] [GPT backup 34 sectors (zero)]
func setupDeviceMapper(parts []actions.PartitionDef, buildDir, dmName string) ([]string, error) {
	cleanupStaleDeviceMapper(dmName)

	const sectorSize = uint64(512)
	alignmentSectors := uint64(2048) // 1MB alignment for GPT header
	gptBackupSectors := uint64(34)

	// Create GPT header image (1MB)
	gptPath := filepath.Join(buildDir, "gpt.img")
	gptSize := alignmentSectors * sectorSize
	f, err := os.Create(gptPath)
	if err != nil {
		return nil, fmt.Errorf("creating gpt.img: %w", err)
	}
	if err := f.Truncate(int64(gptSize)); err != nil {
		f.Close()
		return nil, fmt.Errorf("sizing gpt.img: %w", err)
	}
	f.Close()

	// Loop-attach gpt.img
	gptLoop, err := runOutput("losetup", "--find", "--show", gptPath)
	if err != nil {
		return nil, fmt.Errorf("attaching gpt.img: %w", err)
	}
	loopDevs := []string{gptLoop}

	// Build dm-linear table
	var tableLines []string
	sectorOffset := uint64(0)

	// GPT header segment
	tableLines = append(tableLines, fmt.Sprintf("%d %d linear %s 0", sectorOffset, alignmentSectors, gptLoop))
	sectorOffset += alignmentSectors

	// Track partition layout for GPT creation
	var partEntries []partEntry

	for i, part := range parts {
		imgPath := filepath.Join(buildDir, fmt.Sprintf("%s.img", part.Name))
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			cleanupDeviceMapper(dmName, loopDevs)
			return nil, fmt.Errorf("partition image not found: %s", imgPath)
		}

		loopDev, err := runOutput("losetup", "--find", "--show", imgPath)
		if err != nil {
			cleanupDeviceMapper(dmName, loopDevs)
			return nil, fmt.Errorf("attaching %s: %w", part.Name, err)
		}
		loopDevs = append(loopDevs, loopDev)

		partSectors := part.Size / sectorSize
		tableLines = append(tableLines, fmt.Sprintf("%d %d linear %s 0", sectorOffset, partSectors, loopDev))

		partEntries = append(partEntries, partEntry{
			num:         i + 1,
			startSector: sectorOffset,
			endSector:   sectorOffset + partSectors - 1,
			name:        part.Name,
			filesystem:  part.Filesystem,
			partType:    part.Type,
		})

		sectorOffset += partSectors
	}

	// GPT backup: use dm-zero target (returns zeros, discards writes)
	tableLines = append(tableLines, fmt.Sprintf("%d %d zero", sectorOffset, gptBackupSectors))

	// Create device mapper device
	table := strings.Join(tableLines, "\n")
	cmd := exec.Command(resolveBin("dmsetup"), "create", dmName)
	cmd.Env = vendorEnv()
	cmd.Stdin = strings.NewReader(table)
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		cleanupDeviceMapper(dmName, loopDevs)
		return nil, fmt.Errorf("dmsetup create: %w", err)
	}

	dmDevice := fmt.Sprintf("/dev/mapper/%s", dmName)

	// Write GPT partition table using sfdisk (sector-based, scriptable via stdin)
	if err := writeSfdiskGPT(dmDevice, partEntries); err != nil {
		cleanupDeviceMapper(dmName, loopDevs)
		return nil, fmt.Errorf("creating GPT: %w", err)
	}

	// Inform kernel of partition changes
	run("partprobe", dmDevice)

	return loopDevs, nil
}

// writeSfdiskGPT creates a GPT partition table on the device using sfdisk's
// scriptable stdin format. Partitions are placed at exact sector offsets with
// no alignment adjustment (sizes specified in sectors bypass sfdisk alignment).
func writeSfdiskGPT(device string, entries []partEntry) error {
	var script strings.Builder
	script.WriteString("label: gpt\n")

	for _, pe := range entries {
		size := pe.endSector - pe.startSector + 1
		fmt.Fprintf(&script, "start=%d, size=%d, type=%s, name=%q\n",
			pe.startSector, size, sfdiskTypeAlias(pe.partType), pe.name)
	}

	cmd := exec.Command(resolveBin("sfdisk"), device)
	cmd.Env = vendorEnv()
	cmd.Stdin = strings.NewReader(script.String())
	if out != nil {
		cmd.Stdout = out.LogWriter()
		cmd.Stderr = out.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// sfdiskTypeAlias maps partition type names to sfdisk GPT type aliases.
func sfdiskTypeAlias(partType string) string {
	switch partType {
	case "efi":
		return "uefi"
	case "bios-boot":
		return "bios-boot"
	case "swap":
		return "swap"
	case "home":
		return "home"
	case "raid":
		return "raid"
	case "lvm":
		return "lvm"
	case "microsoft-basic":
		return "microsoft-basic-data"
	case "microsoft-reserved":
		return "microsoft-reserved"
	case "root":
		return "root"
	case "root-verity":
		return "root-verity"
	case "usr":
		return "usr"
	case "usr-verity":
		return "usr-verity"
	default:
		return "linux"
	}
}

// patchBootForSerial temporarily patches the boot partition image to add
// console=ttyS0,115200 to kernel options and remove quiet/splash so the
// guest outputs to the QEMU serial console. Returns a restore function
// that reverts the patch after QEMU exits.
func patchBootForSerial(buildDir string, parts []actions.PartitionDef) (func(), error) {
	noop := func() {}

	// Find the EFI boot partition image
	var bootImg string
	for _, p := range parts {
		if p.Type == "efi" {
			bootImg = filepath.Join(buildDir, fmt.Sprintf("%s.img", p.Name))
			break
		}
	}
	if bootImg == "" {
		return noop, nil
	}

	// Mount the boot image
	tmpDir, err := os.MkdirTemp("", "starforge-serial-*")
	if err != nil {
		return noop, fmt.Errorf("creating temp dir: %w", err)
	}

	if err := run("mount", "-o", "loop", bootImg, tmpDir); err != nil {
		os.Remove(tmpDir)
		return noop, fmt.Errorf("mounting boot image: %w", err)
	}

	// Read loader.conf to find default entry
	loaderPath := filepath.Join(tmpDir, "loader", "loader.conf")
	loaderData, err := os.ReadFile(loaderPath)
	if err != nil {
		run("umount", tmpDir)
		os.Remove(tmpDir)
		return noop, fmt.Errorf("reading loader.conf: %w", err)
	}

	var defaultEntry string
	for _, line := range strings.Split(string(loaderData), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "default") {
			defaultEntry = strings.TrimSpace(strings.TrimPrefix(line, "default"))
			break
		}
	}
	if defaultEntry == "" {
		run("umount", tmpDir)
		os.Remove(tmpDir)
		return noop, nil
	}

	// Read and patch the entry
	entryPath := filepath.Join(tmpDir, "loader", "entries", defaultEntry)
	entryData, err := os.ReadFile(entryPath)
	if err != nil {
		run("umount", tmpDir)
		os.Remove(tmpDir)
		return noop, fmt.Errorf("reading boot entry %s: %w", defaultEntry, err)
	}

	original := string(entryData)
	var patched []string
	for _, line := range strings.Split(original, "\n") {
		if strings.HasPrefix(line, "options") {
			// Remove quiet and splash, add serial console
			opts := strings.TrimPrefix(line, "options")
			opts = strings.TrimSpace(opts)
			var filtered []string
			for _, opt := range strings.Fields(opts) {
				if opt != "quiet" && opt != "splash" {
					filtered = append(filtered, opt)
				}
			}
			filtered = append(filtered, "console=tty0", "console=ttyS0,115200")
			line = "options " + strings.Join(filtered, " ")
		}
		patched = append(patched, line)
	}

	if err := os.WriteFile(entryPath, []byte(strings.Join(patched, "\n")), 0o644); err != nil {
		run("umount", tmpDir)
		os.Remove(tmpDir)
		return noop, fmt.Errorf("writing patched entry: %w", err)
	}

	// Unmount — the changes are flushed to boot.img
	syscall.Sync()
	if err := run("umount", tmpDir); err != nil {
		os.Remove(tmpDir)
		return noop, fmt.Errorf("unmounting boot image: %w", err)
	}
	os.Remove(tmpDir)

	// Return restore function
	restore := func() {
		restoreDir, err := os.MkdirTemp("", "starforge-serial-restore-*")
		if err != nil {
			return
		}
		defer os.Remove(restoreDir)

		if err := run("mount", "-o", "loop", bootImg, restoreDir); err != nil {
			return
		}
		restorePath := filepath.Join(restoreDir, "loader", "entries", defaultEntry)
		if err := os.WriteFile(restorePath, []byte(original), 0o644); err != nil {
			run("umount", restoreDir)
			return
		}
		syscall.Sync()
		run("umount", restoreDir)
	}

	return restore, nil
}

// findOVMF locates the OVMF UEFI firmware file.
// Checks vendored path first, then common system paths.
func findOVMF() (string, error) {
	vendorDir := VendorDir()
	candidates := []string{
		filepath.Join(vendorDir, "usr/share/edk2/x64/OVMF_CODE.4m.fd"),
		filepath.Join(vendorDir, "usr/share/edk2/x64/OVMF_CODE.fd"),
		"/usr/share/edk2/x64/OVMF_CODE.4m.fd",
		"/usr/share/edk2/x64/OVMF_CODE.fd",
		"/usr/share/OVMF/OVMF_CODE.fd",
		"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("OVMF firmware not found — install edk2-ovmf or run 'starforge run' to vendor it")
}

// RunQEMU assembles partition images into a virtual disk via device mapper
// and boots with QEMU.
func RunQEMU(targetName, buildDir, projectDir string, parts []actions.PartitionDef, serial bool, overlayName, bootDisk string, qemuCfg *config.QEMUConfig) error {
	// Vendor run dependencies (OVMF, dmsetup, sfdisk)
	if err := EnsureDeps("run"); err != nil {
		return fmt.Errorf("dependencies: %w", err)
	}

	dmName := deviceMapperName(targetName, projectDir)

	// Determine image directory: named overlay or build dir
	imageDir := buildDir
	if overlayName != "" {
		overlayDir, err := EnsureNamedOverlay(buildDir, overlayName, parts)
		if err != nil {
			return fmt.Errorf("named overlay: %w", err)
		}
		imageDir = overlayDir
	}

	// Verify partition images exist
	for _, part := range parts {
		imgPath := filepath.Join(imageDir, fmt.Sprintf("%s.img", part.Name))
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			return fmt.Errorf("partition image not found: %s — run 'starforge build %s' first", imgPath, targetName)
		}
	}

	// Patch boot entry for serial console before assembling the disk
	if serial {
		restore, err := patchBootForSerial(imageDir, parts)
		if err != nil {
			return fmt.Errorf("patching boot for serial: %w", err)
		}
		defer restore()
	}

	// Set up device mapper (skipped in boot-disk mode)
	var loopDevs []string
	if bootDisk == "" {
		out.Header("Assembling virtual disk")
		out.Styled(
			fmt.Sprintf("  device mapper: %s", dmName),
			fmt.Sprintf("  device mapper: %s", dmName),
		)

		var err error
		loopDevs, err = setupDeviceMapper(parts, imageDir, dmName)
		if err != nil {
			return fmt.Errorf("setting up device mapper: %w", err)
		}
	}
	defer func() {
		if bootDisk == "" {
			cleanupDeviceMapper(dmName, loopDevs)
			// Remove temporary GPT header image created by setupDeviceMapper
			os.Remove(filepath.Join(imageDir, "gpt.img"))
		}
	}()

	// Find OVMF firmware
	ovmfPath, err := findOVMF()
	if err != nil {
		return err
	}
	out.Styled(
		fmt.Sprintf("  OVMF: %s", ovmfPath),
		fmt.Sprintf("  OVMF: %s", ovmfPath),
	)

	// Resolve QEMU options (config overrides → defaults)
	mem := 4096
	cpus := 4
	gpuMem := 512
	display := "gtk,gl=on,show-cursor=off,zoom-to-fit=on"
	cpuModel := "host"
	audioDriver := detectAudioDriver()
	sshPort := 2222

	if qemuCfg != nil {
		if qemuCfg.Memory > 0 {
			mem = qemuCfg.Memory
		}
		if qemuCfg.CPUs > 0 {
			cpus = qemuCfg.CPUs
		}
		if qemuCfg.GPUMemory > 0 {
			gpuMem = qemuCfg.GPUMemory
		}
		if qemuCfg.Display != "" {
			display = qemuCfg.Display
		}
		if qemuCfg.CPU != "" {
			cpuModel = qemuCfg.CPU
		}
		if qemuCfg.Audio != "" {
			audioDriver = qemuCfg.Audio
		}
		if qemuCfg.SSHPort > 0 {
			sshPort = qemuCfg.SSHPort
		}
	}

	out.Styled(
		fmt.Sprintf("  memory: %dM, cpus: %d, gpu: %dM, audio: %s", mem, cpus, gpuMem, audioDriver),
		fmt.Sprintf("  memory: %dM, cpus: %d, gpu: %dM, audio: %s", mem, cpus, gpuMem, audioDriver),
	)

	// Provision additional disks
	if qemuCfg != nil && len(qemuCfg.Disks) > 0 {
		if err := ensureQEMUDisks(buildDir, qemuCfg.Disks); err != nil {
			return err
		}
	}

	// Build QEMU command
	qemuArgs := []string{
		"-m", fmt.Sprintf("%dM", mem),
		"-smp", strconv.Itoa(cpus),
		"-cpu", cpuModel,
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", ovmfPath),
		"-device", fmt.Sprintf("virtio-vga-gl,max_hostmem=%dM,blob=on,edid=on,xres=1920,yres=1080", gpuMem),
		"-display", display,
		"-device", "virtio-rng-pci",
		"-device", "virtio-balloon-pci",
		"-usb", "-device", "usb-tablet",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", sshPort),
		"-device", "virtio-net-pci,netdev=net0",
	}

	// Audio: virtual sound card so the guest PipeWire stack can produce sound
	if audioDriver != "none" {
		qemuArgs = append(qemuArgs,
			"-audiodev", fmt.Sprintf("%s,id=snd0", audioDriver),
			"-device", "intel-hda",
			"-device", "hda-duplex,audiodev=snd0",
		)
	}

	if bootDisk != "" {
		// Boot from a named QEMU disk (e.g. after running the installer)
		bootImg := filepath.Join(buildDir, "disks", bootDisk+".img")
		if _, err := os.Stat(bootImg); os.IsNotExist(err) {
			return fmt.Errorf("boot disk not found: %s — available disks are in .starforge/<target>/disks/", bootImg)
		}
		out.Styled(
			fmt.Sprintf("  boot disk: %s", bootDisk),
			fmt.Sprintf("  boot disk: %s", bootDisk),
		)
		qemuArgs = append(qemuArgs,
			"-drive", fmt.Sprintf("file=%s,format=raw,if=virtio,cache=writeback,aio=threads", bootImg),
		)
	} else {
		// Boot from the device-mapper assembled build disk
		qemuArgs = append(qemuArgs, "-drive", qemuDriveOption(dmName, overlayName))
	}

	// Attach additional disks (skip the boot disk if already attached)
	if qemuCfg != nil {
		for _, disk := range qemuCfg.Disks {
			if disk.Name == bootDisk {
				continue
			}
			imgPath := filepath.Join(buildDir, "disks", disk.Name+".img")
			qemuArgs = append(qemuArgs,
				"-drive", fmt.Sprintf("file=%s,format=raw,if=virtio,cache=writeback,aio=threads", imgPath),
			)
		}
	}

	// Append extra args from config
	if qemuCfg != nil && len(qemuCfg.Args) > 0 {
		qemuArgs = append(qemuArgs, qemuCfg.Args...)
	}

	// Enable KVM if available
	if _, err := os.Stat("/dev/kvm"); err == nil {
		qemuArgs = append([]string{"-enable-kvm"}, qemuArgs...)
	}

	// Serial console: multiplex monitor + serial on stdio so all key
	// inputs (including Ctrl+C) are forwarded to the guest. Use the
	// standard QEMU escape prefix Ctrl+A x to quit.
	if serial {
		qemuArgs = append(qemuArgs, "-serial", "mon:stdio")
	}

	out.Blank()
	out.Header("Booting QEMU")
	out.Styled(
		fmt.Sprintf("  SSH: ssh -p %d localhost", sshPort),
		fmt.Sprintf("  SSH: ssh -p %d localhost", sshPort),
	)
	if serial {
		out.Styled(
			"  Serial: connected to terminal (Ctrl+A x to quit)",
			"  Serial: connected to terminal (Ctrl+A x to quit)",
		)
	}
	out.Blank()

	// Intercept signals so defers run (cleanup device mapper, loops, gpt.img).
	// Without this, Ctrl+C kills the process immediately and resources leak.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Run QEMU interactively
	cmd := exec.Command("qemu-system-x86_64", qemuArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting qemu: %w", err)
	}

	// Forward signals to QEMU so it can shut down gracefully
	go func() {
		if sig, ok := <-sigCh; ok {
			cmd.Process.Signal(sig)
		}
	}()

	err = cmd.Wait()

	// Stop the signal goroutine
	signal.Stop(sigCh)
	close(sigCh)

	if err != nil {
		// Exit code from QEMU is not an error for us (user closed window, Ctrl+C)
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return fmt.Errorf("qemu: %w", err)
	}

	return nil
}

// qemuDriveOption returns the QEMU -drive value. When no overlay is set,
// snapshot=on is used so changes are discarded. With a named overlay, writes
// go directly to the overlay images and persist.
func qemuDriveOption(dmName, overlayName string) string {
	base := fmt.Sprintf("file=/dev/mapper/%s,format=raw,if=virtio,cache=writeback,aio=threads", dmName)
	if overlayName == "" {
		return base + ",snapshot=on"
	}
	return base
}

// detectAudioDriver returns the best available QEMU audio backend for the host.
// Checks for PipeWire first (via runtime socket), then falls back to SDL.
// When running under sudo, checks the invoking user's runtime directory.
func detectAudioDriver() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		// Running with sudo — try the invoking user's runtime dir
		if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
			runtimeDir = "/run/user/" + sudoUID
		}
	}
	if runtimeDir != "" {
		if _, err := os.Stat(filepath.Join(runtimeDir, "pipewire-0")); err == nil {
			return "pipewire"
		}
	}
	return "sdl"
}

// ensureQEMUDisks creates any additional disk images that don't already exist.
// Disk images are sparse files stored at .starforge/<target>/disks/<name>.img.
func ensureQEMUDisks(buildDir string, disks []config.QEMUDisk) error {
	diskDir := filepath.Join(buildDir, "disks")
	if err := os.MkdirAll(diskDir, 0o755); err != nil {
		return fmt.Errorf("creating disk directory: %w", err)
	}

	for _, disk := range disks {
		imgPath := filepath.Join(diskDir, disk.Name+".img")
		if _, err := os.Stat(imgPath); err == nil {
			out.Info("disk: %s (existing)", disk.Name)
			continue
		}

		size, _, err := actions.ParseSize(disk.Size)
		if err != nil {
			return fmt.Errorf("disk %q: %w", disk.Name, err)
		}

		f, err := os.Create(imgPath)
		if err != nil {
			return fmt.Errorf("creating disk %q: %w", disk.Name, err)
		}
		if err := f.Truncate(int64(size)); err != nil {
			f.Close()
			return fmt.Errorf("sizing disk %q: %w", disk.Name, err)
		}
		f.Close()

		out.Info("disk: %s (%s, new)", disk.Name, disk.Size)
	}

	return nil
}

