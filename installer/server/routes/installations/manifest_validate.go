package installations

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/telemetryos/starforge/installer"
)

// validateManifest rejects payload manifests with fields that, if used
// unvalidated to drive `mount`, `mkfs`, `sfdisk`, or filesystem operations,
// could let a tampered installer image escape the target rootfs or inject
// shell metacharacters into the install pipeline.
//
// Defense-in-depth: every field that flows from manifest.json into a
// subprocess argv or filesystem path is checked here, in one place,
// before the runInstallation pipeline runs.
func validateManifest(m *installer.PayloadManifest) error {
	if len(m.Partitions) == 0 {
		return fmt.Errorf("manifest has no partitions")
	}
	seenName := map[string]bool{}
	seenMount := map[string]bool{}
	for i, p := range m.Partitions {
		if err := validatePartitionName(p.Name); err != nil {
			return fmt.Errorf("partitions[%d].name %q: %w", i, p.Name, err)
		}
		if seenName[p.Name] {
			return fmt.Errorf("partitions[%d]: duplicate partition name %q", i, p.Name)
		}
		seenName[p.Name] = true

		if err := validateMountPoint(p.MountPoint); err != nil {
			return fmt.Errorf("partitions[%d] (%s).mount_point %q: %w", i, p.Name, p.MountPoint, err)
		}
		if p.MountPoint != "" {
			if seenMount[p.MountPoint] {
				return fmt.Errorf("partitions[%d]: mount_point %q already used by another partition", i, p.MountPoint)
			}
			seenMount[p.MountPoint] = true
		}

		if err := validateFilesystem(p.Filesystem); err != nil {
			return fmt.Errorf("partitions[%d] (%s).filesystem %q: %w", i, p.Name, p.Filesystem, err)
		}
		if err := validatePartitionType(p.Type); err != nil {
			return fmt.Errorf("partitions[%d] (%s).type %q: %w", i, p.Name, p.Type, err)
		}
		if err := validateImageRef(p.Corona); err != nil {
			return fmt.Errorf("partitions[%d] (%s).corona %q: %w", i, p.Name, p.Corona, err)
		}
	}

	// EFI label flows into `efibootmgr --label`; argv-only (no shell), but
	// keep it printable / bounded length to avoid NVRAM clutter.
	if err := validateEFILabel(m.EFILabel); err != nil {
		return fmt.Errorf("efi_label %q: %w", m.EFILabel, err)
	}
	return nil
}

// validatePartitionName matches the same charset sfdisk + udev are happy
// with for GPT partition labels and that filesystem labels accept. Limits
// to printable ASCII without separators or shell metas.
func validatePartitionName(s string) error {
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	if len(s) > 36 {
		return fmt.Errorf("must be 36 chars or fewer")
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("contains invalid character %q (allow [A-Za-z0-9_-])", r)
		}
	}
	return nil
}

// validateMountPoint accepts an absolute Unix path that filepath.Clean
// leaves unchanged. Empty is allowed (e.g. swap partitions). No "..",
// no "//", no relative paths, no NUL.
func validateMountPoint(s string) error {
	if s == "" {
		return nil
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("contains NUL byte")
	}
	if !strings.HasPrefix(s, "/") {
		return fmt.Errorf("must be absolute (start with /)")
	}
	if filepath.Clean(s) != s {
		return fmt.Errorf("must be a clean path (no .., redundant slashes)")
	}
	// Disallow mount points that would clobber kernel virtual filesystems.
	switch s {
	case "/proc", "/sys", "/dev", "/run":
		return fmt.Errorf("must not target kernel virtual filesystem %q", s)
	}
	return nil
}

// validateFilesystem checks the filesystem name against the set the
// install runtime actually knows how to mkfs / mount.
func validateFilesystem(s string) error {
	switch s {
	case "ext4", "vfat", "fat32", "swap":
		return nil
	case "":
		return fmt.Errorf("must not be empty")
	}
	return fmt.Errorf("unsupported filesystem")
}

// validatePartitionType checks the GPT partition type alias against the
// set isValidPartitionType in actions/resolve.go accepts.
func validatePartitionType(s string) error {
	switch s {
	case "linux", "efi", "xbootldr", "swap", "home", "bios-boot",
		"raid", "lvm", "microsoft-basic", "microsoft-reserved",
		"root", "root-verity", "usr", "usr-verity":
		return nil
	case "":
		return fmt.Errorf("must not be empty")
	}
	return fmt.Errorf("unknown partition type")
}

// validateImageRef restricts payload image filenames to single basenames
// — no path separators, no .., no NUL. The runtime code path-joins this
// with resolvedDir (the payload dir on the installer USB), so a `../`
// would let a malicious manifest reach files outside the payload dir.
func validateImageRef(s string) error {
	if s == "" {
		return nil // empty image means "format an empty filesystem"
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("contains NUL byte")
	}
	if strings.ContainsAny(s, "/\\") {
		return fmt.Errorf("must be a single filename, not a path")
	}
	if s == "." || s == ".." {
		return fmt.Errorf("must not be . or ..")
	}
	if filepath.Base(s) != s {
		return fmt.Errorf("must be a clean basename")
	}
	return nil
}

// validateEFILabel keeps the NVRAM EFI variable label printable and
// bounded. argv-passed to efibootmgr so no shell injection risk; this
// is just hygiene.
func validateEFILabel(s string) error {
	if s == "" {
		return nil
	}
	if len(s) > 64 {
		return fmt.Errorf("must be 64 chars or fewer")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("contains control character")
		}
	}
	return nil
}
