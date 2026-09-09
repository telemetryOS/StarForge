package installer

import "github.com/telemetryos/starforge/platform"

// PayloadManifest describes a bundled target's partition images.
type PayloadManifest struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	EFILabel    string             `json:"efiLabel,omitempty"`
	Platform    platform.Spec      `json:"platform"`
	Partitions  []PayloadPartition `json:"partitions"`
}

// PayloadPartition describes a single partition image within a payload.
type PayloadPartition struct {
	Name       string `json:"name"`
	Filesystem string `json:"filesystem"`
	Size       uint64 `json:"size"`
	MountPoint string `json:"mountPoint"`
	Type       string `json:"type"`
	Grow       bool   `json:"grow"`
	Corona     string `json:"corona"` // filename, e.g. "boot.corona"
}
