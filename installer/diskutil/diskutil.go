package diskutil

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Disk represents a block device.
type Disk struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      uint64 `json:"size"`
	Model     string `json:"model"`
	Transport string `json:"transport"`
}

// lsblkOutput mirrors the JSON structure from lsblk --json.
type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name  string  `json:"name"`
	Path  string  `json:"path"`
	Size  uint64  `json:"size"`
	Model *string `json:"model"`
	Tran  *string `json:"tran"`
	Type  string  `json:"type"`
}

// ListDisks enumerates physical disks using lsblk, excluding the source
// (installer) disk and non-disk devices such as loop and optical drives.
func ListDisks() ([]Disk, error) {
	out, err := exec.Command("lsblk", "-d", "-b", "-J", "-o", "NAME,PATH,SIZE,MODEL,TRAN,TYPE").Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}

	var parsed lsblkOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parsing lsblk output: %w", err)
	}

	sourceDisk, _ := SourceDisk()

	var disks []Disk
	for _, d := range parsed.BlockDevices {
		if d.Type != "disk" {
			continue
		}
		if d.Name == sourceDisk {
			continue
		}

		disk := Disk{
			Name: d.Name,
			Path: d.Path,
			Size: d.Size,
		}
		if d.Model != nil {
			disk.Model = *d.Model
		}
		if d.Tran != nil {
			disk.Transport = *d.Tran
		}
		disks = append(disks, disk)
	}
	return disks, nil
}

// SourceDisk returns the name of the disk the installer is running from
// by resolving the root mount point back to its parent disk.
func SourceDisk() (string, error) {
	// Find the device mounted at /
	srcOut, err := exec.Command("findmnt", "-n", "-o", "SOURCE", "/").Output()
	if err != nil {
		return "", fmt.Errorf("findmnt: %w", err)
	}
	source := strings.TrimSpace(string(srcOut))

	// Strip any subvolume suffix like [/@]
	if idx := strings.Index(source, "["); idx != -1 {
		source = source[:idx]
	}

	// Resolve the partition to its parent disk
	pkOut, err := exec.Command("lsblk", "-n", "-o", "PKNAME", source).Output()
	if err != nil {
		return "", fmt.Errorf("resolving parent disk: %w", err)
	}
	parent := strings.TrimSpace(string(pkOut))
	if parent != "" {
		return parent, nil
	}

	// Source is already a whole disk (not a partition)
	source = strings.TrimPrefix(source, "/dev/")
	return source, nil
}

// GetDisk returns a single disk by name.
func GetDisk(name string) (*Disk, error) {
	disks, err := ListDisks()
	if err != nil {
		return nil, err
	}
	for _, d := range disks {
		if d.Name == name {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("disk %q not found", name)
}

// FormatSize formats a byte count as a human-readable string.
func FormatSize(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
