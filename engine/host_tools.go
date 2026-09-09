package engine

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// HostToolProvider prepares native tools used by the build engine.
type HostToolProvider interface {
	Ensure(groups ...string) error
}

// VendoredHostTools downloads and uses StarForge's host-tool bundle.
type VendoredHostTools struct{}

// Ensure prepares the requested vendored tool groups.
func (VendoredHostTools) Ensure(groups ...string) error {
	return EnsureDeps(groups...)
}

// SystemHostTools uses tools already installed in an isolated build appliance.
type SystemHostTools struct{}

// Ensure checks that the build appliance contains the requested tools.
func (SystemHostTools) Ensure(groups ...string) error {
	required := map[string]bool{}
	for _, group := range groups {
		switch group {
		case "build":
			for _, name := range []string{
				"arch-chroot", "bash", "blkid", "blockdev", "e2fsck", "findmnt",
				"gpg", "gpg-agent", "gpgconf", "losetup", "lsblk", "mkfs.ext4",
				"mkfs.vfat", "mount", "pacman", "pacman-key", "pacstrap", "partprobe",
				"resize2fs", "sfdisk", "tar", "udevadm", "umount", "zstd",
			} {
				required[name] = true
			}
		case "run":
			return fmt.Errorf("target VMs are launched by the host coordinator, not the build appliance")
		default:
			return fmt.Errorf("unknown host tool group %q", group)
		}
	}
	var missing []string
	for name := range required {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("build appliance is missing system tools: %s", strings.Join(missing, ", "))
	}
	return nil
}
