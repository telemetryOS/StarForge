package installer

// PayloadManifest describes a bundled target's partition images.
type PayloadManifest struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	EFILabel    string             `json:"efi_label,omitempty"`
	Partitions  []PayloadPartition `json:"partitions"`
}

// PayloadPartition describes a single partition image within a payload.
type PayloadPartition struct {
	Name       string `json:"name"`
	Filesystem string `json:"filesystem"`
	Size       uint64 `json:"size"`
	MountPoint string `json:"mount_point"`
	Type       string `json:"type"`
	Grow       bool   `json:"grow"`
	Artifact   string `json:"artifact"` // filename, e.g. "boot.corona"
}
