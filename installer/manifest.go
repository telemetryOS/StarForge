package installer

// PayloadManifest describes a bundled target's partition images.
type PayloadManifest struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
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
	Image      string `json:"image"` // filename, e.g. "boot.img"
}

