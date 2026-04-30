package actions

import "github.com/telemetryos/starforge/config"

// BuildContext accumulates all declarative state from the Collect phase.
// Each action appends or replaces fields here; the Execute phase reads them.
type BuildContext struct {
	// System configuration (replace semantics — later layer wins)
	Hostname string
	Locale   string
	Locales  []string
	Timezone string
	Keymap   string

	// Packages (accumulate + remove)
	Packages      []Package
	PackageGroups []LayerGroup

	// Partitions (accumulate + replace-on-name)
	Partitions       []PartitionDef
	PartitionHistory []PartitionSnapshot

	// Users and groups
	Users  []UserDef
	Groups []GroupDef

	// File operations (accumulate)
	FileMkdirs      []FileMkdirOp
	LayerCopies     []LayerCopyOp
	FileCreates     []FileCreateOp
	FileEdits       []FileEditOp
	FileCopies      []FileCopyOp
	FileMoves       []FileMoveOp
	FileLinks       []FileLinkOp
	FileDeletes     []FileDeleteOp

	// Permissions (accumulate)
	FileOwnerships  []FileOwnershipOp
	FilePermissions []FilePermissionOp

	// Services
	Services      ServiceOps
	DefaultTarget string

	// Boot configuration (replace semantics)
	Boot *BootConfig

	// Scripts (accumulate, run in order)
	Scripts []ScriptOp

	// Installer
	InstallPayloads []InstallPayloadDef
	InstallServer   *InstallServerDef
	InstallClient   *InstallClientDef

	// Multi-target composition: targets whose builds are merged into this
	// target's disk image. Populated by the install-embed action. Order is
	// significant for partition merge tie-breaking and for predictable
	// fstab generation.
	InstallEmbeds []string

	// Tracking / history for inspect command
	HostnameHistory      []LayerValue
	LocaleHistory        []LayerValue
	TimezoneHistory      []LayerValue
	KeymapHistory        []LayerValue
	DefaultTargetHistory []LayerValue
	EnableGroups         []LayerGroup
	DisableGroups        []LayerGroup
	MaskGroups           []LayerGroup
	UserEnableGroups     []UserServiceGroup
	UserDisableGroups    []UserServiceGroup

	// Variables and environment
	Vars map[string]string
	Env  map[string]string

	// Internal state
	CurrentLayer     string
	DownloadCacheDir string
	DryRun           bool // skip side effects (layer-run, file reads) for inspect
	Warnings         []string
}

// NewBuildContext creates a BuildContext with all slice and map fields
// initialized to empty (non-nil) values. This avoids surprising nil-vs-empty
// distinctions and makes append() behaviour predictable from the first call.
func NewBuildContext() *BuildContext {
	return &BuildContext{
		// System configuration
		Locales: []string{},

		// Packages
		Packages:      []Package{},
		PackageGroups: []LayerGroup{},

		// Partitions
		Partitions:       []PartitionDef{},
		PartitionHistory: []PartitionSnapshot{},

		// Users and groups
		Users:  []UserDef{},
		Groups: []GroupDef{},

		// File operations
		FileMkdirs:      []FileMkdirOp{},
		LayerCopies:     []LayerCopyOp{},
		FileCreates:     []FileCreateOp{},
		FileEdits:       []FileEditOp{},
		FileCopies:      []FileCopyOp{},
		FileMoves:       []FileMoveOp{},
		FileLinks:       []FileLinkOp{},
		FileDeletes:     []FileDeleteOp{},

		// Permissions
		FileOwnerships:  []FileOwnershipOp{},
		FilePermissions: []FilePermissionOp{},

		// Services
		Services: ServiceOps{},

		// Scripts
		Scripts: []ScriptOp{},

		// Installer
		InstallPayloads: []InstallPayloadDef{},

		// Tracking / history
		HostnameHistory:      []LayerValue{},
		LocaleHistory:        []LayerValue{},
		TimezoneHistory:      []LayerValue{},
		KeymapHistory:        []LayerValue{},
		DefaultTargetHistory: []LayerValue{},
		EnableGroups:         []LayerGroup{},
		DisableGroups:        []LayerGroup{},
		MaskGroups:           []LayerGroup{},
		UserEnableGroups:     []UserServiceGroup{},
		UserDisableGroups:    []UserServiceGroup{},

		// Variables and environment
		Vars: make(map[string]string),
		Env:  make(map[string]string),

		// Internal state
		Warnings: []string{},
	}
}

// Package represents a package to install, optionally pinned to a version.
type Package struct {
	Name    string
	Version string // "" = latest from repos
}

// String returns "name" or "name=version" for display.
func (p Package) String() string {
	if p.Version != "" {
		return p.Name + "=" + p.Version
	}
	return p.Name
}

// --- Operation types ---

// PartitionDef defines a resolved partition for the execute/package phases.
type PartitionDef struct {
	Name       string
	Filesystem string
	Size       uint64
	MountPoint string
	Type       string
	Grow       bool
}

// FileCreateOp creates a file with inline content.
type FileCreateOp struct {
	Path    string
	Content string
	Mode    string
	Layer   string
	Label   string
}

// LayerCopyOp copies a file or directory from the layer to the target.
type LayerCopyOp struct {
	FromPath string
	ToPath   string
	LayerDir string
	Layer    string
	Label    string
}

// FileEditOp edits an existing file in the target.
type FileEditOp struct {
	Path     string
	Content  string
	Insert   string // append, prepend, before, after
	Truncate string // truncate_before, truncate_after
	Pattern  string
	Match    int
	Layer    string
	Label    string
}

// FileCopyOp copies within the target filesystem.
type FileCopyOp struct {
	FromPath string
	ToPath   string
	Layer    string
	Label    string
}

// FileMoveOp moves a file within the target filesystem.
type FileMoveOp struct {
	FromPath string
	ToPath   string
	Layer    string
	Label    string
}

// FileLinkOp creates a symlink or hard link.
type FileLinkOp struct {
	FromPath string // link target (what it points to)
	ToPath   string // link path (where the link is created)
	Type     string // "symbolic" or "hard"
	Layer    string
	Label    string
}

// FileDeleteOp deletes a file or directory.
type FileDeleteOp struct {
	Path      string
	Recursive bool
	Layer     string
	Label     string
}

// FileOwnershipOp changes ownership of a path.
type FileOwnershipOp struct {
	Path      string
	Owner     string
	Group     string
	Recursive bool
	Layer     string
	Label     string
}

// FilePermissionOp changes permissions of a path.
type FilePermissionOp struct {
	Path      string
	Mode      string
	Recursive bool
	Layer     string
	Label     string
}

// FileMkdirOp creates a directory.
type FileMkdirOp struct {
	Path  string
	Owner string
	Group string
	Mode  string
	Layer string
	Label string
}

// ScriptOp runs a script inside the target chroot.
type ScriptOp struct {
	Script   string            // filename (relative to LayerDir)
	Content  string            // inline script content
	User     string            // run as this user
	Env      map[string]string // step-level environment variables
	LayerDir string            // directory containing the script file
	Layer    string
	Label    string
}

// ServiceOps tracks systemd service operations.
type ServiceOps struct {
	Enable      []string
	Disable     []string
	Mask        []string
	UserEnable  []UserServiceOp
	UserDisable []UserServiceOp
}

// UserServiceOp is a user-level systemd unit operation.
type UserServiceOp struct {
	User    string
	Service string
	Layer   string
}

// BootConfig holds systemd-boot configuration.
//
// Loader is a pointer so we can detect "this target did not configure a
// loader: block" — only the host of a multi-target build typically owns
// loader.conf, embeds usually contribute entries only.
type BootConfig struct {
	Loader  *config.BootLoader `json:"loader,omitempty"`
	Entries []config.BootEntry `json:"entries,omitempty"`
	Layer   string             `json:"layer,omitempty"`
}

// UserDef defines a user to create.
type UserDef struct {
	Name       string
	Groups     []string
	Shell      string
	Password   string
	NoPassword bool
	System     bool
	UID        int
	Layer      string
}

// GroupDef defines a group to create.
type GroupDef struct {
	Name   string
	GID    int
	System bool
	Layer  string
}

// LayerGroup tracks which layer added a set of items (for inspect).
type LayerGroup struct {
	Layer string
	Items []string
}

// LayerValue tracks a single value with its source layer (for inspect).
type LayerValue struct {
	Layer string
	Value string
}

// PartitionSnapshot records partition state at a point in time.
type PartitionSnapshot struct {
	Layer      string
	Partitions []PartitionDef
}

// UserServiceGroup tracks user-level service operations by layer.
type UserServiceGroup struct {
	Layer string
	User  string
	Items []string
}

// InstallPayloadDef defines a payload target to bundle.
type InstallPayloadDef struct {
	Target string
	Path   string
	// Partitions optionally restricts which of the target's partition images
	// get bundled. Empty means all partitions.
	Partitions []string
	Layer      string
	Label      string
}

// InstallServerDef configures the installer server.
type InstallServerDef struct {
	Port     int
	Path     string
	Layer    string
	EFILabel string
}

// InstallClientDef configures the installer client TUI.
type InstallClientDef struct {
	AutoLogin  string
	Unattended bool
	Layer      string
}

// RecordPartitionSnapshot appends a snapshot of the current partition state
// to the history, tagged with the current layer name.
func (ctx *BuildContext) RecordPartitionSnapshot() {
	ctx.PartitionHistory = append(ctx.PartitionHistory, PartitionSnapshot{
		Layer:      ctx.CurrentLayer,
		Partitions: copyPartitions(ctx.Partitions),
	})
}
