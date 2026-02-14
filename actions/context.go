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
	Packages      []string
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
	InstallerPayloads []InstallerPayloadDef
	InstallerServer   *InstallerServerDef
	InstallerClient   *InstallerClientDef

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

// NewBuildContext creates an empty BuildContext.
func NewBuildContext() *BuildContext {
	return &BuildContext{
		Services: ServiceOps{},
	}
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
type BootConfig struct {
	Loader  config.BootLoader
	Entries []config.BootEntry
	Layer   string
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

// InstallerPayloadDef defines a payload target to bundle.
type InstallerPayloadDef struct {
	Target string
	Path   string
	Layer  string
	Label  string
}

// InstallerServerDef configures the installer server.
type InstallerServerDef struct {
	Port  int
	Path  string
	Layer string
}

// InstallerClientDef configures the installer client TUI.
type InstallerClientDef struct {
	AutoLogin string
	Layer     string
}
