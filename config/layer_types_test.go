package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// --- Mergeable ---

func TestMergeable_Plain(t *testing.T) {
	var m Mergeable[[]string]
	if err := yaml.Unmarshal([]byte(`[wheel, video]`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Mode != ModeReplace {
		t.Errorf("plain mode = %d, want ModeReplace (%d)", m.Mode, ModeReplace)
	}
	if len(m.Value) != 2 || m.Value[0] != "wheel" || m.Value[1] != "video" {
		t.Errorf("value = %v, want [wheel video]", m.Value)
	}
}

func TestMergeable_Add(t *testing.T) {
	// Edge-OS development layer uses: groups: !add [player]
	var m Mergeable[[]string]
	if err := yaml.Unmarshal([]byte(`!add [player, docker]`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Mode != ModeAdd {
		t.Errorf("!add mode = %d, want ModeAdd (%d)", m.Mode, ModeAdd)
	}
	if len(m.Value) != 2 || m.Value[0] != "player" || m.Value[1] != "docker" {
		t.Errorf("value = %v, want [player docker]", m.Value)
	}
}

func TestMergeable_Remove(t *testing.T) {
	var m Mergeable[[]string]
	if err := yaml.Unmarshal([]byte(`!remove [video]`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Mode != ModeRemove {
		t.Errorf("!remove mode = %d, want ModeRemove (%d)", m.Mode, ModeRemove)
	}
	if len(m.Value) != 1 || m.Value[0] != "video" {
		t.Errorf("value = %v, want [video]", m.Value)
	}
}

// --- TaggedContent ---

func TestTaggedContent_Append(t *testing.T) {
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(`!append "line added at end"`), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "append" {
		t.Errorf("tag = %q, want %q", tc.Tag, "append")
	}
	if tc.Value != "line added at end" {
		t.Errorf("value = %q, want %q", tc.Value, "line added at end")
	}
}

func TestTaggedContent_Prepend(t *testing.T) {
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(`!prepend "line added at start"`), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "prepend" {
		t.Errorf("tag = %q, want %q", tc.Tag, "prepend")
	}
	if tc.Value != "line added at start" {
		t.Errorf("value = %q", tc.Value)
	}
}

func TestTaggedContent_Before(t *testing.T) {
	input := `!before
pattern: "^\\[section\\]"
value: "inserted before match"
match: 1`
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(input), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "before" {
		t.Errorf("tag = %q, want %q", tc.Tag, "before")
	}
	if tc.Pattern != `^\[section\]` {
		t.Errorf("pattern = %q", tc.Pattern)
	}
	if tc.Match != 1 {
		t.Errorf("match = %d, want 1", tc.Match)
	}
	if tc.Value != "inserted before match" {
		t.Errorf("value = %q", tc.Value)
	}
}

func TestTaggedContent_After(t *testing.T) {
	input := `!after
pattern: "^\\[Service\\]"
value: "After=network.target"
match: 0`
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(input), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "after" {
		t.Errorf("tag = %q, want %q", tc.Tag, "after")
	}
	if tc.Pattern != `^\[Service\]` {
		t.Errorf("pattern = %q", tc.Pattern)
	}
	if tc.Match != 0 {
		t.Errorf("match = %d, want 0", tc.Match)
	}
}

func TestTaggedContent_TruncateBefore(t *testing.T) {
	input := `!truncate_before
pattern: "^# START"`
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(input), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "truncate_before" {
		t.Errorf("tag = %q, want %q", tc.Tag, "truncate_before")
	}
	if tc.Pattern != "^# START" {
		t.Errorf("pattern = %q", tc.Pattern)
	}
	if tc.Value != "" {
		t.Errorf("value should be empty for truncate, got %q", tc.Value)
	}
}

func TestTaggedContent_TruncateAfter(t *testing.T) {
	input := `!truncate_after
pattern: "^# END"
match: 2`
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(input), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "truncate_after" {
		t.Errorf("tag = %q, want %q", tc.Tag, "truncate_after")
	}
	if tc.Match != 2 {
		t.Errorf("match = %d, want 2", tc.Match)
	}
}

func TestTaggedContent_Plain(t *testing.T) {
	var tc TaggedContent
	if err := yaml.Unmarshal([]byte(`"plain content"`), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tag != "" {
		t.Errorf("tag = %q, want empty", tc.Tag)
	}
	if tc.Value != "plain content" {
		t.Errorf("value = %q, want %q", tc.Value, "plain content")
	}
}

// --- UnitSection ---

func TestUnitSection_PlainValues(t *testing.T) {
	input := `Description: TelemetryOS Player
After: sway-session.target`
	var us UnitSection
	if err := yaml.Unmarshal([]byte(input), &us); err != nil {
		t.Fatal(err)
	}
	if us["Description"] != "TelemetryOS Player" {
		t.Errorf("Description = %v", us["Description"])
	}
	if us["After"] != "sway-session.target" {
		t.Errorf("After = %v", us["After"])
	}
}

func TestUnitSection_ReplaceTag(t *testing.T) {
	// Edge-OS uses !replace for getty@tty1 autologin drop-in
	input := `ExecStart: !replace "-/sbin/agetty --autologin player"`
	var us UnitSection
	if err := yaml.Unmarshal([]byte(input), &us); err != nil {
		t.Fatal(err)
	}
	rv, ok := us["ExecStart"].(ReplaceValue)
	if !ok {
		t.Fatalf("ExecStart should be ReplaceValue, got %T", us["ExecStart"])
	}
	if rv.Value != "-/sbin/agetty --autologin player" {
		t.Errorf("ReplaceValue.Value = %q", rv.Value)
	}
}

func TestUnitSection_MixedValues(t *testing.T) {
	input := `Type: notify
ExecStart: !replace "/usr/bin/new"
Restart: always`
	var us UnitSection
	if err := yaml.Unmarshal([]byte(input), &us); err != nil {
		t.Fatal(err)
	}
	if us["Type"] != "notify" {
		t.Errorf("Type = %v", us["Type"])
	}
	if us["Restart"] != "always" {
		t.Errorf("Restart = %v", us["Restart"])
	}
	rv, ok := us["ExecStart"].(ReplaceValue)
	if !ok {
		t.Fatalf("ExecStart should be ReplaceValue, got %T", us["ExecStart"])
	}
	if rv.Value != "/usr/bin/new" {
		t.Errorf("ReplaceValue.Value = %q", rv.Value)
	}
}

// --- ExtendsRef ---

func TestExtendsRef_ServiceType(t *testing.T) {
	// Edge-OS uses: extends: {service: getty@tty1}
	input := `service: getty@tty1`
	var e ExtendsRef
	if err := yaml.Unmarshal([]byte(input), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != "service" {
		t.Errorf("Type = %q, want %q", e.Type, "service")
	}
	if e.Name != "getty@tty1" {
		t.Errorf("Name = %q, want %q", e.Name, "getty@tty1")
	}
}

func TestExtendsRef_UnitName(t *testing.T) {
	tests := []struct {
		typ  string
		name string
		want string
	}{
		{"service", "getty@tty1", "getty@tty1.service"},
		{"mount", "data", "data.mount"},
		{"timer", "fstrim", "fstrim.timer"},
		{"socket", "dbus", "dbus.socket"},
		{"slice", "user", "user.slice"},
		{"target", "graphical", "graphical.target"},
		// Name with extension already — keep as-is
		{"service", "getty@tty1.service", "getty@tty1.service"},
	}
	for _, tt := range tests {
		e := ExtendsRef{Type: tt.typ, Name: tt.name}
		got := e.UnitName()
		if got != tt.want {
			t.Errorf("ExtendsRef{%q, %q}.UnitName() = %q, want %q", tt.typ, tt.name, got, tt.want)
		}
	}
}

func TestExtendsRef_UnknownType(t *testing.T) {
	input := `device: sda1`
	var e ExtendsRef
	err := yaml.Unmarshal([]byte(input), &e)
	if err == nil {
		t.Fatal("expected error for unknown unit type")
	}
}

func TestExtendsRef_MultipleKeys(t *testing.T) {
	input := `service: foo
timer: bar`
	var e ExtendsRef
	err := yaml.Unmarshal([]byte(input), &e)
	if err == nil {
		t.Fatal("expected error for multiple keys in extends")
	}
}

// --- Step routing ---

func TestStep_PacmanAdd(t *testing.T) {
	input := `action: pacman-add
packages:
  - base
  - linux`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.Action != "pacman-add" {
		t.Errorf("Action = %q", step.Action)
	}
	if step.PacmanAdd == nil {
		t.Fatal("PacmanAdd is nil")
	}
	if len(step.PacmanAdd.Packages) != 2 {
		t.Errorf("Packages = %v", step.PacmanAdd.Packages)
	}
}

func TestStep_SystemUser(t *testing.T) {
	// Matches Edge-OS base layer player user definition
	input := `action: system-user
name: player
groups: [wheel, video, render, seat, audio, input, data, docker, lp, network]
shell: /bin/bash`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.SystemUser == nil {
		t.Fatal("SystemUser is nil")
	}
	if step.SystemUser.Name != "player" {
		t.Errorf("Name = %q", step.SystemUser.Name)
	}
	if len(step.SystemUser.Groups.Value) != 10 {
		t.Errorf("Groups = %v (len=%d)", step.SystemUser.Groups.Value, len(step.SystemUser.Groups.Value))
	}
	if step.SystemUser.Groups.Mode != ModeReplace {
		t.Errorf("Groups.Mode = %d, want ModeReplace", step.SystemUser.Groups.Mode)
	}
}

func TestStep_SystemUserMergeAdd(t *testing.T) {
	// Matches Edge-OS development layer staff override: groups: !add [player]
	input := `action: system-user
name: staff
shell: /usr/bin/fish
no_password: true
groups: !add [player]`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.SystemUser == nil {
		t.Fatal("SystemUser is nil")
	}
	if step.SystemUser.Groups.Mode != ModeAdd {
		t.Errorf("Groups.Mode = %d, want ModeAdd (%d)", step.SystemUser.Groups.Mode, ModeAdd)
	}
	if len(step.SystemUser.Groups.Value) != 1 || step.SystemUser.Groups.Value[0] != "player" {
		t.Errorf("Groups.Value = %v", step.SystemUser.Groups.Value)
	}
	if !step.SystemUser.NoPassword {
		t.Error("NoPassword should be true")
	}
}

func TestStep_SystemdService(t *testing.T) {
	// Matches Edge-OS player layer inline service definition
	input := `action: systemd-service
name: player
user: player
enable: true
unit:
  Description: TelemetryOS Player
  After: sway-session.target
  BindsTo: sway-session.target
service:
  Type: notify
  Restart: always
install:
  WantedBy: sway-session.target`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.SystemdService == nil {
		t.Fatal("SystemdService is nil")
	}
	if step.SystemdService.Name != "player" {
		t.Errorf("Name = %q", step.SystemdService.Name)
	}
	if step.SystemdService.User != "player" {
		t.Errorf("User = %q", step.SystemdService.User)
	}
	if !step.SystemdService.Enable {
		t.Error("Enable should be true")
	}
	if step.SystemdService.UnitSec["Description"] != "TelemetryOS Player" {
		t.Errorf("UnitSec.Description = %v", step.SystemdService.UnitSec["Description"])
	}
	if step.SystemdService.Service["Type"] != "notify" {
		t.Errorf("Service.Type = %v", step.SystemdService.Service["Type"])
	}
}

func TestStep_SystemdServiceExtends(t *testing.T) {
	// Matches Edge-OS base layer autologin drop-in for getty@tty1
	input := `action: systemd-service
name: autologin.conf
extends:
  service: getty@tty1
service:
  ExecStart: !replace "-/sbin/agetty --autologin player"
`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.SystemdService == nil {
		t.Fatal("SystemdService is nil")
	}
	if step.SystemdService.Extends == nil {
		t.Fatal("Extends is nil")
	}
	if step.SystemdService.Extends.Type != "service" {
		t.Errorf("Extends.Type = %q", step.SystemdService.Extends.Type)
	}
	if step.SystemdService.Extends.Name != "getty@tty1" {
		t.Errorf("Extends.Name = %q", step.SystemdService.Extends.Name)
	}
	rv, ok := step.SystemdService.Service["ExecStart"].(ReplaceValue)
	if !ok {
		t.Fatalf("Service.ExecStart should be ReplaceValue, got %T", step.SystemdService.Service["ExecStart"])
	}
	if rv.Value != "-/sbin/agetty --autologin player" {
		t.Errorf("ReplaceValue = %q", rv.Value)
	}
}

func TestStep_PartitionAdd(t *testing.T) {
	// Matches Edge-OS base layer partition layout
	input := `action: partition-add
partitions:
  - name: boot
    filesystem: vfat
    size: 1G
    mount_point: /boot
    type: efi
  - name: data
    filesystem: ext4
    size: 256M+
    mount_point: /data
    type: linux`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.PartitionAdd == nil {
		t.Fatal("PartitionAdd is nil")
	}
	if len(step.PartitionAdd.Partitions) != 2 {
		t.Fatalf("Partitions len = %d", len(step.PartitionAdd.Partitions))
	}
	boot := step.PartitionAdd.Partitions[0]
	if boot.Name != "boot" || boot.Filesystem != "vfat" || boot.Size != "1G" || boot.Type != "efi" {
		t.Errorf("boot partition = %+v", boot)
	}
	data := step.PartitionAdd.Partitions[1]
	if data.Size != "256M+" {
		t.Errorf("data partition size = %q, want %q", data.Size, "256M+")
	}
}

func TestStep_UnknownAction(t *testing.T) {
	input := `action: unknown-action`
	var step Step
	err := yaml.Unmarshal([]byte(input), &step)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestStep_MissingAction(t *testing.T) {
	input := `packages: [base]`
	var step Step
	err := yaml.Unmarshal([]byte(input), &step)
	if err == nil {
		t.Fatal("expected error for missing action field")
	}
}

func TestStep_Label(t *testing.T) {
	input := `action: pacman-add
label: Base system packages
packages: [base]`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.Label != "Base system packages" {
		t.Errorf("Label = %q", step.Label)
	}
}

func TestStep_BootInstall(t *testing.T) {
	// Matches Edge-OS base layer boot configuration
	input := `action: systemd-boot-install
loader:
  default: arch.conf
  timeout: 0
  editor: false
entries:
  - name: arch.conf
    title: TelemetryOS Edge
    linux: /vmlinuz-linux
    initrd: /initramfs-linux.img
    options: rw quiet splash rootflags=noatime,commit=600 audit=0 noresume`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.SystemdBootInstall == nil {
		t.Fatal("SystemdBootInstall is nil")
	}
	if step.SystemdBootInstall.Loader == nil {
		t.Fatal("Loader is nil")
	}
	if step.SystemdBootInstall.Loader.Default != "arch.conf" {
		t.Errorf("Loader.Default = %q", step.SystemdBootInstall.Loader.Default)
	}
	if step.SystemdBootInstall.Loader.Timeout != 0 {
		t.Errorf("Loader.Timeout = %d", step.SystemdBootInstall.Loader.Timeout)
	}
	if step.SystemdBootInstall.Loader.Editor {
		t.Error("Loader.Editor should be false")
	}
	if len(step.SystemdBootInstall.Entries) != 1 {
		t.Fatalf("Entries len = %d", len(step.SystemdBootInstall.Entries))
	}
	entry := step.SystemdBootInstall.Entries[0]
	if entry.Title != "TelemetryOS Edge" {
		t.Errorf("Entry.Title = %q", entry.Title)
	}
}

func TestStep_Run(t *testing.T) {
	// Matches Edge-OS development layer script
	input := `action: run
user: staff
script: |
  #!/bin/bash
  curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.Run == nil {
		t.Fatal("Run is nil")
	}
	if step.Run.User != "staff" {
		t.Errorf("User = %q", step.Run.User)
	}
	if step.Run.Script == "" {
		t.Error("Script should not be empty")
	}
}

func TestStep_InstallPayload(t *testing.T) {
	// Matches Edge-OS installer layer
	input := `action: install-payload
target: device
path: /images/device`
	var step Step
	if err := yaml.Unmarshal([]byte(input), &step); err != nil {
		t.Fatal(err)
	}
	if step.InstallPayload == nil {
		t.Fatal("InstallPayload is nil")
	}
	if step.InstallPayload.Target != "device" {
		t.Errorf("Target = %q", step.InstallPayload.Target)
	}
	if step.InstallPayload.Path != "/images/device" {
		t.Errorf("Path = %q", step.InstallPayload.Path)
	}
}
