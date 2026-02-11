package diskutil

import (
	"encoding/json"
	"fmt"
	"os/exec"
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
}

// ListDisks enumerates block devices using lsblk.
func ListDisks() ([]Disk, error) {
	out, err := exec.Command("lsblk", "-d", "-b", "-J", "-o", "NAME,PATH,SIZE,MODEL,TRAN").Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}

	var parsed lsblkOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parsing lsblk output: %w", err)
	}

	var disks []Disk
	for _, d := range parsed.BlockDevices {
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
