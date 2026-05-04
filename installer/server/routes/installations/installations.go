package installations

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RobertWHurst/navaros"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/corona"
	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/installer"
	"github.com/telemetryos/starforge/installer/diskutil"
	"github.com/telemetryos/starforge/installer/server/routes/payloads"
)

// Installation tracks the state of an in-progress or completed installation.
type Installation struct {
	ID        string    `json:"id"`
	Payload   string    `json:"payload"`
	Disk      string    `json:"disk"`
	Status    string    `json:"status"`   // pending, partitioning, copying, bootloader, configuring, complete, failed
	Progress  float64   `json:"progress"` // 0.0 - 1.0
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`

	mu  sync.Mutex
	log []string
}

func (inst *Installation) addLog(msg string) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.log = append(inst.log, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), sanitizeLogLine(msg)))
}

// sanitizeLogLine strips ANSI escape sequences and other control bytes from
// subprocess output so a malicious hook script can't inject terminal-escape
// sequences (title rewrites, fake prompts, etc.) into the install log shown
// to operators in the TUI.
func sanitizeLogLine(s string) string {
	if s == "" {
		return s
	}
	// Fast path: no ESC byte → nothing to strip.
	hasEsc := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == 0x1b || (b < 0x20 && b != '\t') || b == 0x7f {
			hasEsc = true
			break
		}
	}
	if !hasEsc {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 0x1b:
			// ESC: skip the sequence. Two common multi-byte forms:
			//   ESC [ … letter   (CSI — terminal control)
			//   ESC ] … BEL|ST   (OSC — title rewrites etc.)
			// Anything else (stray ESC followed by an innocent byte)
			// drops only the ESC; the following byte is preserved.
			if i+1 >= len(s) {
				return b.String()
			}
			next := s[i+1]
			switch next {
			case '[':
				i += 2 // consume ESC + [
				for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e) {
					i++
				}
				// i is at the final byte; loop's i++ skips past it.
			case ']':
				i += 2 // consume ESC + ]
				for i < len(s) {
					if s[i] == 0x07 {
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
						i++ // consume ESC; loop will consume \
						break
					}
					i++
				}
			default:
				// Stray ESC — drop it but let `next` flow normally
				// through the next loop iteration.
			}
		case c == '\t' || c == '\n':
			b.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			// Other C0 controls and DEL — drop silently.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func (inst *Installation) setStatus(status string, progress float64) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.Status == "failed" {
		return
	}
	inst.Status = status
	inst.Progress = progress
}

func (inst *Installation) isFailed() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.Status == "failed"
}

func (inst *Installation) complete() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.Status == "failed" {
		return false
	}
	inst.Status = "complete"
	inst.Progress = 1.0
	return true
}

func (inst *Installation) fail(err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.Status = "failed"
	inst.Error = err.Error()
	inst.log = append(inst.log, fmt.Sprintf("[%s] ERROR: %s", time.Now().Format("15:04:05"), err))
}

// Manager manages installation lifecycle.
type Manager struct {
	mu            sync.Mutex
	runMu         sync.Mutex
	installations map[string]*Installation
	activeDisks   map[string]bool // disks currently being installed to
	payloadDir    string
	nextID        int
}

// NewManager creates an installation manager.
func NewManager(payloadDir string) *Manager {
	return &Manager{
		installations: make(map[string]*Installation),
		activeDisks:   make(map[string]bool),
		payloadDir:    payloadDir,
	}
}

// Router returns a router handling installation routes.
func Router() *navaros.Router {
	r := navaros.NewRouter()
	r.Post("/installations", create)
	r.Get("/installations", list)
	r.Get("/installations/:id", get)
	r.Get("/installations/:id/log", getLog)
	r.Delete("/installations/:id", cancel)
	return r
}

func create(ctx *navaros.Context) {
	manager := ctx.MustGet("manager").(*Manager)

	var req struct {
		Payload string `json:"payload"`
		Disk    string `json:"disk"`
	}
	if err := ctx.UnmarshalRequestBody(&req); err != nil {
		ctx.Status = http.StatusBadRequest
		ctx.Body = map[string]string{"error": "invalid request body"}
		return
	}

	if req.Payload == "" || req.Disk == "" {
		ctx.Status = http.StatusBadRequest
		ctx.Body = map[string]string{"error": "payload and disk are required"}
		return
	}

	// Verify payload exists
	resolvedDir, err := payloads.ResolvePayloadDir(manager.payloadDir, req.Payload)
	if err != nil {
		ctx.Status = http.StatusNotFound
		ctx.Body = map[string]string{"error": fmt.Sprintf("payload %q not found", req.Payload)}
		return
	}
	manifestPath := filepath.Join(resolvedDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		ctx.Status = http.StatusNotFound
		ctx.Body = map[string]string{"error": fmt.Sprintf("payload %q not found", req.Payload)}
		return
	}

	// Verify disk is available
	disk, err := diskutil.GetDisk(req.Disk)
	if err != nil {
		ctx.Status = http.StatusNotFound
		ctx.Body = map[string]string{"error": fmt.Sprintf("disk: %v", err)}
		return
	}

	// Load manifest and verify the disk is large enough
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		ctx.Status = http.StatusInternalServerError
		ctx.Body = map[string]string{"error": "reading manifest"}
		return
	}
	var manifest installer.PayloadManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		ctx.Status = http.StatusInternalServerError
		ctx.Body = map[string]string{"error": "parsing manifest"}
		return
	}

	var requiredSize uint64
	for _, p := range manifest.Partitions {
		requiredSize += p.Size
	}
	// GPT overhead: 1MB header + 34 sectors backup ≈ 2MB
	requiredSize += 2 * 1024 * 1024

	if disk.Size < requiredSize {
		ctx.Status = http.StatusBadRequest
		ctx.Body = map[string]string{
			"error": fmt.Sprintf("disk %s (%s) is too small — payload requires %s",
				disk.Path, diskutil.FormatSize(disk.Size), diskutil.FormatSize(requiredSize)),
		}
		return
	}

	inst, err := manager.create(req.Payload, req.Disk)
	if err != nil {
		ctx.Status = http.StatusConflict
		ctx.Body = map[string]string{"error": err.Error()}
		return
	}
	go manager.runInstallation(inst, disk)

	ctx.Status = http.StatusCreated
	ctx.Body = inst
}

func list(ctx *navaros.Context) {
	manager := ctx.MustGet("manager").(*Manager)

	manager.mu.Lock()
	defer manager.mu.Unlock()

	var result []*Installation
	for _, inst := range manager.installations {
		result = append(result, inst)
	}

	// Sort by start time
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.Before(result[j].StartedAt)
	})

	ctx.Status = http.StatusOK
	ctx.Body = result
}

func get(ctx *navaros.Context) {
	manager := ctx.MustGet("manager").(*Manager)
	id := ctx.Params().Get("id")

	inst := manager.get(id)
	if inst == nil {
		ctx.Status = http.StatusNotFound
		ctx.Body = map[string]string{"error": "installation not found"}
		return
	}

	ctx.Status = http.StatusOK
	ctx.Body = inst
}

func getLog(ctx *navaros.Context) {
	manager := ctx.MustGet("manager").(*Manager)
	id := ctx.Params().Get("id")

	inst := manager.get(id)
	if inst == nil {
		ctx.Status = http.StatusNotFound
		ctx.Body = map[string]string{"error": "installation not found"}
		return
	}

	offset := 0
	if offsetStr := ctx.Query().Get("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			ctx.Status = http.StatusBadRequest
			ctx.Body = map[string]string{"error": "offset must be a non-negative integer"}
			return
		}
		offset = v
	}

	inst.mu.Lock()
	var lines []string
	if offset < len(inst.log) {
		lines = inst.log[offset:]
	}
	newOffset := len(inst.log)
	inst.mu.Unlock()

	ctx.Status = http.StatusOK
	ctx.Body = map[string]any{
		"lines":  lines,
		"offset": newOffset,
	}
}

func cancel(ctx *navaros.Context) {
	manager := ctx.MustGet("manager").(*Manager)
	id := ctx.Params().Get("id")

	inst := manager.get(id)
	if inst == nil {
		ctx.Status = http.StatusNotFound
		ctx.Body = map[string]string{"error": "installation not found"}
		return
	}

	inst.mu.Lock()
	wasPending := inst.Status != "complete" && inst.Status != "failed"
	if wasPending {
		inst.Status = "failed"
		inst.Error = "cancelled by user"
	}
	inst.mu.Unlock()

	ctx.Status = http.StatusOK
	ctx.Body = inst
}

func (m *Manager) create(payload, disk string) (*Installation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeDisks[disk] {
		return nil, fmt.Errorf("disk %q already has an active installation", disk)
	}

	m.nextID++
	id := fmt.Sprintf("%d", m.nextID)

	inst := &Installation{
		ID:        id,
		Payload:   payload,
		Disk:      disk,
		Status:    "pending",
		StartedAt: time.Now(),
	}
	m.installations[id] = inst
	m.activeDisks[disk] = true
	return inst, nil
}

func (m *Manager) get(id string) *Installation {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.installations[id]
}

// releaseDisk marks a disk as no longer actively being installed to.
func (m *Manager) releaseDisk(disk string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.activeDisks, disk)
}

// runInstallation executes the full installation pipeline.
func (m *Manager) runInstallation(inst *Installation, disk *diskutil.Disk) {
	defer m.releaseDisk(inst.Disk)

	// Installation logging currently captures process-global stdout/stderr
	// while engine helpers run. Serialize installs so logs cannot cross-wire
	// between two disks and stdout/stderr restoration stays deterministic.
	m.runMu.Lock()
	defer m.runMu.Unlock()

	if inst.isFailed() {
		return
	}

	// Redirect stdout/stderr to the installation log so that engine command
	// output (sfdisk, mkfs, mount, etc.) is visible in the TUI.
	pr, pw, pipeErr := os.Pipe()
	if pipeErr == nil {
		origStdout, origStderr := os.Stdout, os.Stderr
		os.Stdout = pw
		os.Stderr = pw

		done := make(chan struct{})
		go func() {
			scanner := bufio.NewScanner(pr)
			for scanner.Scan() {
				inst.addLog(scanner.Text())
			}
			close(done)
		}()

		defer func() {
			os.Stdout = origStdout
			os.Stderr = origStderr
			pw.Close()
			<-done
			pr.Close()
		}()
	}

	// Resolve payload directory (flat or nested layout)
	resolvedDir, err := payloads.ResolvePayloadDir(m.payloadDir, inst.Payload)
	if err != nil {
		inst.fail(fmt.Errorf("resolving payload: %w", err))
		return
	}

	// Load manifest
	manifestData, err := os.ReadFile(filepath.Join(resolvedDir, "manifest.json"))
	if err != nil {
		inst.fail(fmt.Errorf("reading manifest: %w", err))
		return
	}

	var manifest installer.PayloadManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		inst.fail(fmt.Errorf("parsing manifest: %w", err))
		return
	}

	// Validate every untrusted manifest field before letting it influence
	// mount/format/Corona file writes. The manifest comes from the payload USB,
	// which is conceptually attacker-controlled (e.g. a tampered installer
	// image), so a bogus mount_point: "../.." or a name with shell metas
	// must NOT escape rootfs or compose into an unsafe argv.
	if err := validateManifest(&manifest); err != nil {
		inst.fail(fmt.Errorf("invalid manifest: %w", err))
		return
	}

	inst.addLog(fmt.Sprintf("Installing %s to %s (%s, %s)",
		manifest.Name, disk.Path, disk.Model, diskutil.FormatSize(disk.Size)))

	// Convert manifest partitions to PartitionDefs for the engine
	var parts []partDef
	for _, p := range manifest.Partitions {
		parts = append(parts, partDef{
			name:       p.Name,
			filesystem: p.Filesystem,
			size:       p.Size,
			mountPoint: p.MountPoint,
			partType:   p.Type,
			grow:       p.Grow,
			corona:     p.Corona,
		})
	}

	// Lifecycle hook: pre-partition. Runs before any disk modification —
	// last chance to abort, confirm wipe, or sanity-check the layout.
	if err := runInstallHooks("pre-partition", "", resolvedDir, inst); err != nil {
		runInstallHooks("on-failure", "", resolvedDir, inst)
		inst.fail(fmt.Errorf("pre-partition hook: %w", err))
		return
	}
	if inst.isFailed() {
		return
	}

	// Phase 1: Partition the disk (GPT via sfdisk, no formatting)
	inst.setStatus("partitioning", 0.1)
	inst.addLog(fmt.Sprintf("Partitioning %s", disk.Path))

	engineParts := toEngineParts(parts)
	resolved, err := engine.PartitionDevice(engineParts, disk.Path)
	if err != nil {
		inst.fail(fmt.Errorf("partitioning: %w", err))
		return
	}

	// Phase 2: Write partition images directly to devices.
	// Empty partitions are formatted fresh. Grow partitions are expanded
	// after the image is written. fstab and boot entries are regenerated
	// in phase 3 so UUIDs always match the actual target disk.
	totalParts := len(parts)
	for i, p := range parts {
		if inst.isFailed() {
			return
		}
		progress := 0.1 + (0.8 * float64(i) / float64(totalParts))
		inst.setStatus("copying", progress)

		partDev := engine.PartitionPath(disk.Path, i+1)

		if p.corona == "" {
			// No image — format an empty filesystem
			inst.addLog(fmt.Sprintf("Formatting %s (%s)", p.name, p.filesystem))
			if err := formatPartition(partDev, p.filesystem, p.name); err != nil {
				inst.fail(fmt.Errorf("formatting %s: %w", p.name, err))
				return
			}
			continue
		}

		inst.addLog(fmt.Sprintf("Writing %s (%s)", p.name, diskutil.FormatSize(p.size)))
		if err := engine.UnmountDevice(disk.Path); err != nil {
			inst.fail(fmt.Errorf("unmounting target partitions before writing %s: %w", p.name, err))
			return
		}

		if p.corona != filepath.Base(p.corona) || p.corona == "." || p.corona == ".." {
			inst.fail(fmt.Errorf("invalid corona path in manifest: %q", p.corona))
			return
		}
		coronaPath := filepath.Join(resolvedDir, p.corona)
		if err := writePartitionImage(coronaPath, partDev); err != nil {
			inst.fail(fmt.Errorf("writing %s: %w", p.name, err))
			return
		}

		// If the target partition is larger than the source image
		// (grow partitions), expand the filesystem to fill it.
		if resolved[i].Size > p.size {
			inst.addLog(fmt.Sprintf("Resizing %s to %s", p.name, diskutil.FormatSize(resolved[i].Size)))
			if err := expandFilesystem(partDev, p.filesystem); err != nil {
				inst.fail(fmt.Errorf("expanding %s: %w", p.name, err))
				return
			}
		}
	}

	// Lifecycle hook: post-write. All partition images are flashed and
	// expanded; rootfs not yet mounted. Useful for image verification or
	// raw-byte tweaks before the configuration block runs.
	if err := runInstallHooks("post-write", "", resolvedDir, inst); err != nil {
		runInstallHooks("on-failure", "", resolvedDir, inst)
		inst.fail(fmt.Errorf("post-write hook: %w", err))
		return
	}
	if inst.isFailed() {
		return
	}

	// Phase 3: Post-install configuration — regenerate fstab with UUIDs from
	// the actual target disk and install the bootloader. Boot entries' root UUIDs
	// are baked into the images at build time.
	inst.setStatus("configuring", 0.95)
	inst.addLog("Configuring fstab and bootloader")

	rootfs, err := os.MkdirTemp("", "starforge-install-rootfs-*")
	if err != nil {
		inst.fail(fmt.Errorf("creating temp rootfs: %w", err))
		return
	}
	defer os.RemoveAll(rootfs)

	mt := engine.NewMountTable(rootfs)
	var mounts []engine.PartitionMount
	for i, p := range parts {
		mounts = append(mounts, engine.PartitionMount{
			Source:     engine.PartitionPath(disk.Path, i+1),
			MountPoint: p.mountPoint,
		})
	}
	if err := mt.MountAll(mounts); err != nil {
		inst.fail(fmt.Errorf("mounting for configuration: %w", err))
		return
	}

	if err := engine.EnsureChrootDirs(rootfs); err != nil {
		mt.Unmount()
		inst.fail(fmt.Errorf("creating chroot dirs: %w", err))
		return
	}

	// Boot entries' root=UUID values are baked in at build time and travel
	// through image copy unchanged. The runtime install only needs to drop the
	// bootloader binary onto the ESP.
	if err := engine.InstallSystemdBoot(toEngineParts(parts), rootfs); err != nil {
		mt.Unmount()
		inst.fail(fmt.Errorf("installing bootloader: %w", err))
		return
	}

	if err := engine.GenerateFstab(toEngineParts(parts), rootfs); err != nil {
		mt.Unmount()
		inst.fail(fmt.Errorf("generating fstab: %w", err))
		return
	}

	// Regenerate the initramfs on the target so the autodetect hook picks
	// up the actual hardware (storage drivers, etc.) and plymouth hooks
	// are included. The build-time initramfs only has modules for the
	// build machine.
	inst.addLog("Regenerating initramfs")
	if err := engine.ChrootRun(rootfs, "mkinitcpio", "-P"); err != nil {
		mt.Unmount()
		inst.fail(fmt.Errorf("regenerating initramfs: %w", err))
		return
	}

	// Lifecycle hook: post-install. fstab/bootloader/mkinitcpio are done;
	// every target partition is still mounted under rootfs at its declared
	// mount point. This is where Edge-OS's restore-image staging script
	// runs (mounts the recovery + fallback-recovery subdirs and copies
	// payload images into them).
	if err := runInstallHooks("post-install", rootfs, resolvedDir, inst); err != nil {
		// Run on-failure BEFORE unmounting so any cleanup hook still
		// sees the target rootfs mounted.
		runInstallHooks("on-failure", rootfs, resolvedDir, inst)
		mt.Unmount()
		inst.fail(fmt.Errorf("post-install hook: %w", err))
		return
	}

	mt.Unmount()

	// Set the installed OS as the EFI boot target so the device boots
	// into the installed OS on next reboot rather than back into the
	// installer USB.
	if err := setEFIBootTarget(inst, disk.Path, parts, manifest.EFILabel); err != nil {
		inst.addLog(fmt.Sprintf("Warning: could not set EFI boot target: %v", err))
	}

	if inst.complete() {
		inst.addLog("Installation complete")
	}
}

// partDef holds partition info from the manifest during installation.
type partDef struct {
	name       string
	filesystem string
	size       uint64
	mountPoint string
	partType   string
	grow       bool
	corona     string
}

func toEngineParts(parts []partDef) []actions.PartitionDef {
	var result []actions.PartitionDef
	for _, p := range parts {
		result = append(result, toEnginePart(p))
	}
	return result
}

func toEnginePart(p partDef) actions.PartitionDef {
	return actions.PartitionDef{
		Name:       p.name,
		Filesystem: p.filesystem,
		Size:       p.size,
		MountPoint: p.mountPoint,
		Type:       p.partType,
		Grow:       p.grow,
	}
}

// writePartitionImage writes a Corona file to a block device.
func writePartitionImage(coronaPath, partDev string) error {
	return corona.Convert(context.Background(), coronaPath, partDev, corona.Options{})
}

// formatPartition formats a partition with the given filesystem.
func formatPartition(partDev, filesystem, name string) error {
	switch filesystem {
	case "vfat", "fat32":
		return exec.Command("mkfs.vfat", "-F", "32", partDev).Run()
	case "ext4":
		return exec.Command("mkfs.ext4", "-F", "-L", name, partDev).Run()
	case "swap":
		return exec.Command("mkswap", "-L", name, partDev).Run()
	default:
		return fmt.Errorf("unsupported filesystem: %s", filesystem)
	}
}

// expandFilesystem grows the filesystem on partDev to fill available space.
// This is needed when the target partition is larger than the source image
// (e.g. grow partitions that expanded to fill available disk space).
// Returns an error if the resize fails; unsupported filesystems are silently skipped.
func expandFilesystem(partDev, filesystem string) error {
	switch filesystem {
	case "ext4":
		if err := exec.Command("e2fsck", "-f", "-y", partDev).Run(); err != nil {
			return fmt.Errorf("e2fsck %s: %w", partDev, err)
		}
		if err := exec.Command("resize2fs", partDev).Run(); err != nil {
			return fmt.Errorf("resize2fs %s: %w", partDev, err)
		}
	}
	return nil
}

// setEFIBootTarget creates an EFI boot entry for the installed OS and sets
// it as BootNext and first in BootOrder. This ensures the device boots into
// the installed OS on next reboot rather than back into the installer USB.
//
// bootctl install (run inside arch-chroot during installation) creates NVRAM
// entries with VenHw device paths instead of real HD() paths because the ESP
// is mounted at a temp directory. The firmware can't reliably resolve VenHw
// entries, so we create a proper entry using efibootmgr --create with the
// actual disk device and partition number.
func setEFIBootTarget(inst *Installation, diskPath string, parts []partDef, label string) error {
	// Check if EFI variables are accessible
	if _, err := os.Stat("/sys/firmware/efi/efivars"); err != nil {
		return nil // not an EFI system or no access
	}

	// Find the EFI partition number (1-based)
	efiPartNum := 0
	for i, p := range parts {
		if p.partType == "efi" {
			efiPartNum = i + 1
			break
		}
	}
	if efiPartNum == 0 {
		return nil // no EFI partition
	}

	if label == "" {
		label = "Linux Boot Manager"
	}

	// Create a proper boot entry with a real HD() device path referencing
	// the target disk's EFI partition and systemd-boot loader.
	out, err := exec.Command("efibootmgr",
		"--create",
		"--disk", diskPath,
		"--part", strconv.Itoa(efiPartNum),
		"--loader", `\EFI\systemd\systemd-bootx64.efi`,
		"--label", label,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating boot entry: %s: %w", string(out), err)
	}

	// efibootmgr --create adds the new entry to the front of BootOrder
	// and prints the updated listing. Parse the new BootOrder to find the
	// entry number (it's the first one).
	var targetNum string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "BootOrder:") {
			orderStr := strings.TrimSpace(strings.TrimPrefix(line, "BootOrder:"))
			entries := strings.Split(orderStr, ",")
			if len(entries) > 0 {
				targetNum = strings.TrimSpace(entries[0])
			}
			break
		}
	}

	if targetNum == "" {
		inst.addLog("Created boot entry but could not determine entry number")
		return nil
	}

	inst.addLog(fmt.Sprintf("Created EFI boot entry Boot%s (%s)", targetNum, label))

	// Also set BootNext as a safety net for the immediate next reboot.
	if err := exec.Command("efibootmgr", "--bootnext", targetNum).Run(); err != nil {
		return fmt.Errorf("setting BootNext: %w", err)
	}

	return nil
}
