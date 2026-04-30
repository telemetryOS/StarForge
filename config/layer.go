package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// unitTypeExtensions maps unit type names to their file extensions.
var unitTypeExtensions = map[string]string{
	"service": ".service",
	"mount":   ".mount",
	"timer":   ".timer",
	"socket":  ".socket",
	"slice":   ".slice",
	"target":  ".target",
}

// ExtendsRef identifies the parent unit for a systemd drop-in file.
//
// YAML usage:
//
//	extends:
//	  service: getty@tty1
//
// This creates a drop-in in /etc/systemd/system/getty@tty1.service.d/
type ExtendsRef struct {
	Type string // unit type: service, mount, timer, socket, slice, target
	Name string // unit name (extension added automatically from type)
}

func (e *ExtendsRef) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]string
	if err := value.Decode(&m); err != nil {
		return fmt.Errorf("extends must be a mapping like {service: getty@tty1}: %w", err)
	}
	if len(m) != 1 {
		return fmt.Errorf("extends must have exactly one unit type key, got %d", len(m))
	}
	for k, v := range m {
		e.Type = k
		e.Name = v
	}
	if _, ok := unitTypeExtensions[e.Type]; !ok {
		return fmt.Errorf("extends: unknown unit type %q", e.Type)
	}
	return nil
}

// UnitName returns the full unit name with extension (e.g. "getty@tty1.service").
func (e ExtendsRef) UnitName() string {
	name := e.Name
	if filepath.Ext(name) == "" {
		name += unitTypeExtensions[e.Type]
	}
	return name
}

// MergeMode controls how a Mergeable field combines with existing values.
type MergeMode int

const (
	ModeReplace MergeMode = iota // default — replace existing values entirely
	ModeAdd                      // !add — append to existing values
	ModeRemove                   // !remove — remove from existing values
)

// Mergeable wraps any type with explicit merge control via YAML tags.
// Actions check the Mode field to decide how to combine values across layers.
//
// YAML usage:
//
//	groups: [wheel, video]          # replace (default)
//	groups: !add [docker, render]   # add to existing
//	groups: !remove [video]         # remove from existing
//
// Works with any YAML-decodable type (slices, maps, etc.):
//
//	Groups   Mergeable[[]string]          // slice with merge control
//	Settings Mergeable[map[string]string] // map with merge control
//
// For !remove on map fields, the YAML value is always a list of keys to remove
// (not a map), so the action must handle the type difference.
type Mergeable[T any] struct {
	Value T
	Mode  MergeMode
}

func (m *Mergeable[T]) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!add":
		m.Mode = ModeAdd
		value.Tag = ""
	case "!remove":
		m.Mode = ModeRemove
		value.Tag = ""
	}
	return value.Decode(&m.Value)
}

func (m Mergeable[T]) MarshalYAML() (any, error) {
	return m.Value, nil
}

// TaggedContent wraps file-edit content with an optional YAML tag that specifies
// the edit mode and any parameters (pattern, match) the mode requires.
//
// Simple modes — tag on a scalar string:
//
//	content: !append |
//	  line added at end
//	content: !prepend |
//	  line added at start
//
// Pattern modes — tag on a mapping with pattern, optional match, and value:
//
//	content: !before
//	  pattern: "^\\[section\\]"
//	  value: |
//	    inserted before match
//	content: !after
//	  pattern: "^\\[section\\]"
//	  match: 1
//	  value: |
//	    inserted after first match
//
// Truncate modes — tag on a mapping with pattern and optional match (no value):
//
//	content: !truncate_before
//	  pattern: "^# START"
//	content: !truncate_after
//	  pattern: "^# END"
//	  match: 2
type TaggedContent struct {
	Value   string // content text (empty for truncate)
	Tag     string // append, prepend, before, after, truncate_before, truncate_after
	Pattern string // regex pattern (for before, after, truncate_before, truncate_after)
	Match   int    // match limit (0 = all for insert, default 1 for truncate)
}

func (tc *TaggedContent) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!append", "!prepend":
		tc.Tag = value.Tag[1:] // strip "!"
		value.Tag = ""
		return value.Decode(&tc.Value)

	case "!before", "!after", "!truncate_before", "!truncate_after":
		tc.Tag = value.Tag[1:]
		value.Tag = ""
		var m struct {
			Pattern string `yaml:"pattern"`
			Match   int    `yaml:"match,omitempty"`
			Value   string `yaml:"value,omitempty"`
		}
		if err := value.Decode(&m); err != nil {
			return fmt.Errorf("%s: %w", tc.Tag, err)
		}
		tc.Pattern = m.Pattern
		tc.Match = m.Match
		tc.Value = m.Value
		return nil

	default:
		// No tag — plain string (backward compat with explicit insert/truncate fields)
		return value.Decode(&tc.Value)
	}
}

// ReplaceValue signals that a systemd directive should be cleared before being set.
// Used in drop-in files where the parent unit's value must be reset first.
//
// YAML usage (in a systemd unit section map):
//
//	service:
//	  ExecStart: !replace "-/sbin/agetty --autologin player"
//
// Renders as:
//
//	ExecStart=
//	ExecStart=-/sbin/agetty --autologin player
type ReplaceValue struct {
	Value any
}

// UnitSection is a map type for systemd unit section fields that supports
// the !replace YAML tag for clear-then-set directive patterns.
type UnitSection map[string]any

func (us *UnitSection) UnmarshalYAML(value *yaml.Node) error {
	*us = make(UnitSection)
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node for unit section")
	}
	for i := 0; i < len(value.Content)-1; i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]

		var key string
		if err := keyNode.Decode(&key); err != nil {
			return err
		}

		if valNode.Tag == "!replace" {
			valNode.Tag = ""
			var val any
			if err := valNode.Decode(&val); err != nil {
				return err
			}
			(*us)[key] = ReplaceValue{Value: val}
		} else {
			var val any
			if err := valNode.Decode(&val); err != nil {
				return err
			}
			(*us)[key] = val
		}
	}
	return nil
}

// Layer represents a layer.yaml configuration.
type Layer struct {
	Steps []Step `yaml:"steps"`

	// Dir is the absolute path to the layer directory (not serialized).
	Dir string `yaml:"-"`
}

// Partition defines a single partition in a partition layout.
type Partition struct {
	Name       string `yaml:"name"`
	Filesystem string `yaml:"filesystem"`
	Size       string `yaml:"size"`
	MountPoint string `yaml:"mount_point"`
	Type       string `yaml:"type,omitempty"`
}

// Step represents a single action within a layer.
// Each action's fields are in a typed sub-struct. Only one is populated
// per step, determined by the action name during YAML unmarshaling.
type Step struct {
	Action          string `yaml:"action"`
	Label           string `yaml:"label,omitempty"`
	LayerSource     string `yaml:"layer_source,omitempty"`
	LayerScript     string `yaml:"layer_script,omitempty"`
	LayerScriptPath string `yaml:"layer_script_path,omitempty"`

	// One field per action type (only one populated)
	PacmanAdd          *PacmanAddStep          `yaml:"-"`
	PacmanRemove       *PacmanRemoveStep       `yaml:"-"`
	FileCreate         *FileCreateStep         `yaml:"-"`
	FileEdit           *FileEditStep           `yaml:"-"`
	FileCopy           *FileCopyStep           `yaml:"-"`
	FileMove           *FileMoveStep           `yaml:"-"`
	FileDelete         *FileDeleteStep         `yaml:"-"`
	FileLink           *FileLinkStep           `yaml:"-"`
	FilePermissions    *FilePermissionsStep    `yaml:"-"`
	FileOwnership      *FileOwnershipStep      `yaml:"-"`
	FileMkdir          *FileMkdirStep          `yaml:"-"`
	SystemUser         *SystemUserStep         `yaml:"-"`
	SystemGroup        *SystemGroupStep        `yaml:"-"`
	SystemHostname     *SystemHostnameStep     `yaml:"-"`
	SystemLocale       *SystemLocaleStep       `yaml:"-"`
	SystemTimezone     *SystemTimezoneStep     `yaml:"-"`
	SystemKeymap       *SystemKeymapStep       `yaml:"-"`
	SystemdTarget      *SystemdTargetStep      `yaml:"-"`
	SystemdBootInstall *SystemdBootInstallStep `yaml:"-"`
	PartitionAdd       *PartitionAddStep       `yaml:"-"`
	PartitionRemove    *PartitionRemoveStep    `yaml:"-"`
	PartitionChange    *PartitionChangeStep    `yaml:"-"`
	Run                *RunStep                `yaml:"-"`

	SystemdService *SystemdServiceStep `yaml:"-"`
	SystemdMount   *SystemdMountStep   `yaml:"-"`
	SystemdTimer   *SystemdTimerStep   `yaml:"-"`
	SystemdSocket  *SystemdSocketStep  `yaml:"-"`
	SystemdSlice   *SystemdSliceStep   `yaml:"-"`

	InstallServer  *InstallServerStep  `yaml:"-"`
	InstallClient  *InstallClientStep  `yaml:"-"`
	InstallPayload *InstallPayloadStep `yaml:"-"`
	LayerRun       *LayerRunStep       `yaml:"-"`
	InstallEmbed   *InstallEmbedStep   `yaml:"-"`
}

// UnmarshalYAML routes YAML fields to the correct typed sub-struct based on action.
func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Action          string `yaml:"action"`
		Label           string `yaml:"label"`
		LayerSource     string `yaml:"layer_source"`
		LayerScript     string `yaml:"layer_script"`
		LayerScriptPath string `yaml:"layer_script_path"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.Action == "" {
		return fmt.Errorf("action is required")
	}
	s.Action = raw.Action
	s.Label = raw.Label
	s.LayerSource = raw.LayerSource
	s.LayerScript = raw.LayerScript
	s.LayerScriptPath = raw.LayerScriptPath

	// Validate layer_script fields
	if raw.LayerScript != "" && raw.LayerScriptPath != "" {
		return fmt.Errorf("layer_script and layer_script_path are mutually exclusive")
	}
	if (raw.LayerScript != "" || raw.LayerScriptPath != "") && raw.LayerSource == "" {
		return fmt.Errorf("layer_script requires layer_source to be set")
	}

	switch raw.Action {
	case "pacman-add":
		s.PacmanAdd = &PacmanAddStep{}
		return value.Decode(s.PacmanAdd)
	case "pacman-remove":
		s.PacmanRemove = &PacmanRemoveStep{}
		return value.Decode(s.PacmanRemove)
	case "file-create":
		s.FileCreate = &FileCreateStep{}
		return value.Decode(s.FileCreate)
	case "file-edit":
		s.FileEdit = &FileEditStep{}
		return value.Decode(s.FileEdit)
	case "file-copy":
		s.FileCopy = &FileCopyStep{}
		return value.Decode(s.FileCopy)
	case "file-move":
		s.FileMove = &FileMoveStep{}
		return value.Decode(s.FileMove)
	case "file-delete":
		s.FileDelete = &FileDeleteStep{}
		return value.Decode(s.FileDelete)
	case "file-link":
		s.FileLink = &FileLinkStep{}
		return value.Decode(s.FileLink)
	case "file-permissions":
		s.FilePermissions = &FilePermissionsStep{}
		return value.Decode(s.FilePermissions)
	case "file-ownership":
		s.FileOwnership = &FileOwnershipStep{}
		return value.Decode(s.FileOwnership)
	case "file-mkdir":
		s.FileMkdir = &FileMkdirStep{}
		return value.Decode(s.FileMkdir)
	case "system-user":
		s.SystemUser = &SystemUserStep{}
		return value.Decode(s.SystemUser)
	case "system-group":
		s.SystemGroup = &SystemGroupStep{}
		return value.Decode(s.SystemGroup)
	case "system-hostname":
		s.SystemHostname = &SystemHostnameStep{}
		return value.Decode(s.SystemHostname)
	case "system-locale":
		s.SystemLocale = &SystemLocaleStep{}
		return value.Decode(s.SystemLocale)
	case "system-timezone":
		s.SystemTimezone = &SystemTimezoneStep{}
		return value.Decode(s.SystemTimezone)
	case "system-keymap":
		s.SystemKeymap = &SystemKeymapStep{}
		return value.Decode(s.SystemKeymap)
	case "systemd-target":
		s.SystemdTarget = &SystemdTargetStep{}
		return value.Decode(s.SystemdTarget)
	case "systemd-boot-install":
		s.SystemdBootInstall = &SystemdBootInstallStep{}
		return value.Decode(s.SystemdBootInstall)
	case "partition-add":
		s.PartitionAdd = &PartitionAddStep{}
		return value.Decode(s.PartitionAdd)
	case "partition-remove":
		s.PartitionRemove = &PartitionRemoveStep{}
		return value.Decode(s.PartitionRemove)
	case "partition-change":
		s.PartitionChange = &PartitionChangeStep{}
		return value.Decode(s.PartitionChange)
	case "run":
		s.Run = &RunStep{}
		return value.Decode(s.Run)
	case "systemd-service":
		s.SystemdService = &SystemdServiceStep{}
		return value.Decode(s.SystemdService)
	case "systemd-mount":
		s.SystemdMount = &SystemdMountStep{}
		return value.Decode(s.SystemdMount)
	case "systemd-timer":
		s.SystemdTimer = &SystemdTimerStep{}
		return value.Decode(s.SystemdTimer)
	case "systemd-socket":
		s.SystemdSocket = &SystemdSocketStep{}
		return value.Decode(s.SystemdSocket)
	case "systemd-slice":
		s.SystemdSlice = &SystemdSliceStep{}
		return value.Decode(s.SystemdSlice)
	case "install-server":
		s.InstallServer = &InstallServerStep{}
		return value.Decode(s.InstallServer)
	case "install-client":
		s.InstallClient = &InstallClientStep{}
		return value.Decode(s.InstallClient)
	case "install-payload":
		s.InstallPayload = &InstallPayloadStep{}
		return value.Decode(s.InstallPayload)
	case "layer-run":
		s.LayerRun = &LayerRunStep{}
		return value.Decode(s.LayerRun)
	case "install-embed":
		s.InstallEmbed = &InstallEmbedStep{}
		return value.Decode(s.InstallEmbed)
	default:
		return fmt.Errorf("unknown action: %q", raw.Action)
	}
}

// Typed step structs — each action gets only the fields it needs.

type PacmanAddStep struct {
	Action   string   `yaml:"action"`
	Packages []string `yaml:"packages"`
}

type PacmanRemoveStep struct {
	Action   string   `yaml:"action"`
	Packages []string `yaml:"packages"`
}

type FileCreateStep struct {
	Action    string `yaml:"action"`
	Path      string `yaml:"path"`
	Content   string `yaml:"content,omitempty"`
	LayerPath string `yaml:"layer_path,omitempty"`
	Mode      string `yaml:"mode,omitempty"`
}

// FileEditStep modifies an existing file using tagged edit operations.
// The content field supports YAML tags: !append, !prepend, !before, !after,
// !truncate_before, !truncate_after. The !before and !after tags require a
// non-empty pattern. Legacy insert/truncate/pattern fields are deprecated.
type FileEditStep struct {
	Action    string        `yaml:"action"`
	Path      string        `yaml:"path"`
	Content   TaggedContent `yaml:"content,omitempty"`
	LayerPath string        `yaml:"layer_path,omitempty"`
	Insert    string        `yaml:"insert,omitempty"` // deprecated: use content tags (!append, !prepend, !before, !after)
	Truncate  string        `yaml:"truncate,omitempty"`
	Pattern   string        `yaml:"pattern,omitempty"`
	Match     int           `yaml:"match,omitempty"`
}

type FileCopyStep struct {
	Action   string `yaml:"action"`
	FromPath string `yaml:"from_path"`
	ToPath   string `yaml:"to_path"`
}

type FileMoveStep struct {
	Action   string `yaml:"action"`
	FromPath string `yaml:"from_path"`
	ToPath   string `yaml:"to_path"`
}

type FileDeleteStep struct {
	Action    string `yaml:"action"`
	Path      string `yaml:"path"`
	Recursive bool   `yaml:"recursive,omitempty"`
}

type FileLinkStep struct {
	Action   string `yaml:"action"`
	FromPath string `yaml:"from_path"`
	ToPath   string `yaml:"to_path"`
	Type     string `yaml:"type,omitempty"` // "symbolic" (default) or "hard"
}

type FilePermissionsStep struct {
	Action    string `yaml:"action"`
	Path      string `yaml:"path"`
	Mode      string `yaml:"mode"`
	Recursive bool   `yaml:"recursive,omitempty"`
}

type FileOwnershipStep struct {
	Action    string `yaml:"action"`
	Path      string `yaml:"path"`
	Owner     string `yaml:"owner,omitempty"`
	Group     string `yaml:"group,omitempty"`
	Recursive bool   `yaml:"recursive,omitempty"`
}

type FileMkdirStep struct {
	Action string `yaml:"action"`
	Path   string `yaml:"path"`
	Owner  string `yaml:"owner,omitempty"`
	Group  string `yaml:"group,omitempty"`
	Mode   string `yaml:"mode,omitempty"`
}

// SystemUserStep creates or modifies a system user.
// Groups supports !add, !remove, and !replace merge modes for cross-layer updates.
// Use system: true for system accounts (no home dir). Use no_password: true to
// allow passwordless login (e.g. for autologin users).
type SystemUserStep struct {
	Action     string              `yaml:"action"`
	Name       string              `yaml:"name"`
	Groups     Mergeable[[]string] `yaml:"groups,omitempty"`
	Shell      string              `yaml:"shell,omitempty"`
	Password   string              `yaml:"password,omitempty"`
	NoPassword bool                `yaml:"no_password,omitempty"`
	System     bool                `yaml:"system,omitempty"`
	UID        int                 `yaml:"uid,omitempty"`
}

type SystemGroupStep struct {
	Action string `yaml:"action"`
	Name   string `yaml:"name"`
	GID    int    `yaml:"gid,omitempty"`
	System bool   `yaml:"system,omitempty"`
}

type SystemHostnameStep struct {
	Action   string `yaml:"action"`
	Hostname string `yaml:"hostname"`
}

type SystemLocaleStep struct {
	Action  string   `yaml:"action"`
	Locale  string   `yaml:"locale,omitempty"`
	Locales []string `yaml:"locales,omitempty"`
}

type SystemTimezoneStep struct {
	Action   string `yaml:"action"`
	Timezone string `yaml:"timezone"`
}

type SystemKeymapStep struct {
	Action string `yaml:"action"`
	Keymap string `yaml:"keymap"`
}

type SystemdTargetStep struct {
	Action string `yaml:"action"`
	// Set-default mode
	Target string `yaml:"target,omitempty"`
	// Create-target mode
	Name      string      `yaml:"name,omitempty"`
	User      string      `yaml:"user,omitempty"`
	Enable    bool        `yaml:"enable,omitempty"`
	Disable   bool        `yaml:"disable,omitempty"`
	Mask      bool        `yaml:"mask,omitempty"`
	UnitSec   UnitSection `yaml:"unit,omitempty"`
	Install   UnitSection `yaml:"install,omitempty"`
	LayerPath string      `yaml:"layer_path,omitempty"`
}

type SystemdBootInstallStep struct {
	Action  string      `yaml:"action"`
	Loader  *BootLoader `yaml:"loader,omitempty"`
	Entries []BootEntry `yaml:"entries,omitempty"`
}

// PartitionAddStep adds one or more partitions to the build target.
// Each partition specifies filesystem, size, mount point, and optional GPT type.
// The After field inserts partitions after a named existing partition.
type PartitionAddStep struct {
	Action     string      `yaml:"action"`
	Partitions []Partition `yaml:"partitions"`
	After      string      `yaml:"after,omitempty"`
}

type PartitionRemoveStep struct {
	Action string `yaml:"action"`
	Name   string `yaml:"name"`
}

type PartitionChangeStep struct {
	Action     string `yaml:"action"`
	Name       string `yaml:"name"`
	Filesystem string `yaml:"filesystem,omitempty"`
	Size       string `yaml:"size,omitempty"`
	MountPoint string `yaml:"mount_point,omitempty"`
	PartType   string `yaml:"type,omitempty"`
}

// RunStep executes a shell script in the built OS via arch-chroot.
// Provide either script (inline content) or script_path (file relative to the
// layer directory, or a URL). The optional user field runs the script as that
// user; env injects additional environment variables.
type RunStep struct {
	Action     string            `yaml:"action"`
	Script     string            `yaml:"script,omitempty"`      // inline script content
	ScriptPath string            `yaml:"script_path,omitempty"` // file path (relative to layer dir) or URL
	User       string            `yaml:"user,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
}

// SystemdServiceStep creates or configures a systemd .service unit.
// Supports standalone creation (with UnitSec/Service/Install sections or
// layer_path), enable/disable/mask operations, and drop-in overrides via extends.
type SystemdServiceStep struct {
	Action    string      `yaml:"action"`
	Name      string      `yaml:"name,omitempty"`
	User      string      `yaml:"user,omitempty"`
	Enable    bool        `yaml:"enable,omitempty"`
	Disable   bool        `yaml:"disable,omitempty"`
	Mask      bool        `yaml:"mask,omitempty"`
	Extends   *ExtendsRef `yaml:"extends,omitempty"`
	LayerPath string      `yaml:"layer_path,omitempty"`
	UnitSec   UnitSection `yaml:"unit,omitempty"`
	Service   UnitSection `yaml:"service,omitempty"`
	Install   UnitSection `yaml:"install,omitempty"`
}

type SystemdMountStep struct {
	Action    string      `yaml:"action"`
	Name      string      `yaml:"name,omitempty"`
	User      string      `yaml:"user,omitempty"`
	Enable    bool        `yaml:"enable,omitempty"`
	Disable   bool        `yaml:"disable,omitempty"`
	Mask      bool        `yaml:"mask,omitempty"`
	Extends   *ExtendsRef `yaml:"extends,omitempty"`
	LayerPath string      `yaml:"layer_path,omitempty"`
	UnitSec   UnitSection `yaml:"unit,omitempty"`
	Mount     UnitSection `yaml:"mount,omitempty"`
	Install   UnitSection `yaml:"install,omitempty"`
}

type SystemdTimerStep struct {
	Action    string      `yaml:"action"`
	Name      string      `yaml:"name,omitempty"`
	User      string      `yaml:"user,omitempty"`
	Enable    bool        `yaml:"enable,omitempty"`
	Disable   bool        `yaml:"disable,omitempty"`
	Mask      bool        `yaml:"mask,omitempty"`
	Extends   *ExtendsRef `yaml:"extends,omitempty"`
	LayerPath string      `yaml:"layer_path,omitempty"`
	UnitSec   UnitSection `yaml:"unit,omitempty"`
	Timer     UnitSection `yaml:"timer,omitempty"`
	Install   UnitSection `yaml:"install,omitempty"`
}

type SystemdSocketStep struct {
	Action    string      `yaml:"action"`
	Name      string      `yaml:"name,omitempty"`
	User      string      `yaml:"user,omitempty"`
	Enable    bool        `yaml:"enable,omitempty"`
	Disable   bool        `yaml:"disable,omitempty"`
	Mask      bool        `yaml:"mask,omitempty"`
	Extends   *ExtendsRef `yaml:"extends,omitempty"`
	LayerPath string      `yaml:"layer_path,omitempty"`
	UnitSec   UnitSection `yaml:"unit,omitempty"`
	Socket    UnitSection `yaml:"socket,omitempty"`
	Install   UnitSection `yaml:"install,omitempty"`
}

type SystemdSliceStep struct {
	Action    string      `yaml:"action"`
	Name      string      `yaml:"name,omitempty"`
	User      string      `yaml:"user,omitempty"`
	Enable    bool        `yaml:"enable,omitempty"`
	Disable   bool        `yaml:"disable,omitempty"`
	Mask      bool        `yaml:"mask,omitempty"`
	Extends   *ExtendsRef `yaml:"extends,omitempty"`
	LayerPath string      `yaml:"layer_path,omitempty"`
	UnitSec   UnitSection `yaml:"unit,omitempty"`
	Slice     UnitSection `yaml:"slice,omitempty"`
	Install   UnitSection `yaml:"install,omitempty"`
}

type InstallServerStep struct {
	Action   string `yaml:"action"`
	Port     int    `yaml:"port,omitempty"`
	Path     string `yaml:"path"`
	EFILabel string `yaml:"efi_label,omitempty"`
}

type InstallClientStep struct {
	Action     string `yaml:"action"`
	AutoLogin  string `yaml:"auto_login,omitempty"`
	Unattended bool   `yaml:"unattended,omitempty"`
}

type InstallPayloadStep struct {
	Action string `yaml:"action"`
	Target string `yaml:"target"`
	Path   string `yaml:"path"`
	// Partitions optionally restricts which of the target's partition images
	// get bundled. Empty (default) means all of the target's partitions.
	// Used to seed a recovery rootfs with just the active boot/root images
	// it can flash.
	Partitions []string `yaml:"partitions,omitempty"`
}

type LayerRunStep struct {
	Action     string            `yaml:"action"`
	Script     string            `yaml:"script,omitempty"`
	ScriptPath string            `yaml:"script_path,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
}

// InstallEmbedStep marks another target's full build as a contributor to this
// target's disk image. The named target builds independently (own rootfs
// overlay, own actions); its partition declarations are merged with this
// target's at packaging time and its overlay contributes files to whichever
// partitions it mounts.
//
// Pairs with InstallPayloadStep — both are forms of "this target depends on
// another target." install-embed merges overlays into the same disk image;
// install-payload bundles the dep's packaged images for the runtime installer.
type InstallEmbedStep struct {
	Action string `yaml:"action"`
	Target string `yaml:"target"`
}

// BootLoader represents systemd-boot loader configuration.
type BootLoader struct {
	Default string `yaml:"default" json:"default"`
	Timeout int    `yaml:"timeout" json:"timeout"`
	Editor  bool   `yaml:"editor" json:"editor"`
}

// BootEntry represents a systemd-boot entry.
//
// Kernel names a kernel package (mkinitcpio convention, e.g. "linux"). The
// engine derives filenames as vmlinuz-<kernel> and initramfs-<kernel>.img.
//
// Extended controls which boot partition the entry lives on. When unset
// (nil), the engine defaults to the XBOOTLDR partition if one is declared
// in the partition layout, else the ESP. Setting it explicitly forces
// the placement: false → ESP (used for a fallback recovery entry that
// must stay on the frozen ESP even though XBOOTLDR is active), true →
// XBOOTLDR (errors if no XBOOTLDR partition is declared).
//
// Path is an optional override for where the kernel/initrd files should
// live in the rootfs overlay. It must be a subpath of the entry's
// partition mount point. When omitted, defaults to that mount point
// itself. The engine auto-stages the kernel/initrd files at the resolved
// destination if they aren't already there, copying from the canonical
// pacman locations (/boot/vmlinuz-<kernel>, /boot/initramfs-<kernel>.img).
type BootEntry struct {
	Name     string `yaml:"name" json:"name"`
	Title    string `yaml:"title" json:"title"`
	SortKey  string `yaml:"sortKey,omitempty" json:"sortKey,omitempty"`
	Kernel   string `yaml:"kernel" json:"kernel"`
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	Options  string `yaml:"options" json:"options"`
	Extended *bool  `yaml:"extended,omitempty" json:"extended,omitempty"`
}

const LayerFile = "layer.yaml"

// LoadLayer reads and parses a layer.yaml from the given directory.
// cacheDir is used for URL-based !include resolution (empty string disables URL includes).
func LoadLayer(dir string, cacheDir ...string) (*Layer, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving layer path: %w", err)
	}

	path := filepath.Join(absDir, LayerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s in %s: %w", LayerFile, absDir, err)
	}

	// Parse into node tree for !include pre-processing
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s in %s: %w", LayerFile, absDir, err)
	}

	// Resolve !include tags
	cd := ""
	if len(cacheDir) > 0 {
		cd = cacheDir[0]
	}
	if err := ResolveIncludes(&doc, absDir, cd); err != nil {
		return nil, fmt.Errorf("resolving includes in %s: %w", absDir, err)
	}

	// Decode resolved tree into typed struct
	var layer Layer
	if err := doc.Decode(&layer); err != nil {
		return nil, fmt.Errorf("parsing %s in %s: %w", LayerFile, absDir, err)
	}

	layer.Dir = absDir

	return &layer, nil
}

// RawLayer represents a layer loaded from layer.yaml with unprocessed step
// nodes. This supports variable substitution before steps are decoded.
type RawLayer struct {
	Dir       string            `yaml:"-"`
	Vars      map[string]string `yaml:"vars,omitempty"`
	Imports   []string          `yaml:"imports,omitempty"`
	Exports   []string          `yaml:"exports,omitempty"`
	StepNodes []*yaml.Node      `yaml:"-"`
}

// LoadLayerRaw reads a layer.yaml and returns it with raw step nodes.
// The raw nodes allow variable substitution before type-specific decoding.
func LoadLayerRaw(dir string, cacheDir string) (*RawLayer, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving layer path: %w", err)
	}

	path := filepath.Join(absDir, LayerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s in %s: %w", LayerFile, absDir, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s in %s: %w", LayerFile, absDir, err)
	}

	if err := ResolveIncludes(&doc, absDir, cacheDir); err != nil {
		return nil, fmt.Errorf("resolving includes in %s: %w", absDir, err)
	}

	// Extract top-level fields from the document node
	raw := &RawLayer{Dir: absDir}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return raw, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return raw, nil
	}

	for i := 0; i < len(root.Content)-1; i += 2 {
		key := root.Content[i].Value
		val := root.Content[i+1]

		switch key {
		case "vars":
			raw.Vars = make(map[string]string)
			if err := val.Decode(&raw.Vars); err != nil {
				return nil, fmt.Errorf("decoding vars: %w", err)
			}
		case "imports":
			if err := val.Decode(&raw.Imports); err != nil {
				return nil, fmt.Errorf("decoding imports: %w", err)
			}
		case "exports":
			if err := val.Decode(&raw.Exports); err != nil {
				return nil, fmt.Errorf("decoding exports: %w", err)
			}
		case "steps":
			if val.Kind == yaml.SequenceNode {
				raw.StepNodes = val.Content
			}
		}
	}

	return raw, nil
}

// DecodeStep decodes a single yaml.Node into a typed Step struct.
func DecodeStep(node *yaml.Node) (Step, error) {
	var step Step
	if err := node.Decode(&step); err != nil {
		return Step{}, err
	}
	return step, nil
}
