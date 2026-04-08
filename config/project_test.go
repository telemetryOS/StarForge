package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProject_Validate_Valid(t *testing.T) {
	p := &Project{
		Name: "test-project",
		Targets: map[string]Target{
			"device": {Layers: []string{"layers/base"}},
		},
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestProject_Validate_NoName(t *testing.T) {
	p := &Project{
		Targets: map[string]Target{
			"device": {Layers: []string{"layers/base"}},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestProject_Validate_NoTargets(t *testing.T) {
	p := &Project{Name: "test", Targets: map[string]Target{}}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for empty targets")
	}
}

func TestProject_Validate_EmptyLayers(t *testing.T) {
	p := &Project{
		Name: "test",
		Targets: map[string]Target{
			"device": {Layers: []string{}},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for target with no layers")
	}
}

func TestProject_BuildDir(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}
	got := p.BuildDir()
	want := "/home/user/Edge-OS/.starforge"
	if got != want {
		t.Errorf("BuildDir() = %q, want %q", got, want)
	}
}

func TestProject_TargetBuildDir(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}
	got := p.TargetBuildDir("device")
	want := "/home/user/Edge-OS/.starforge/device"
	if got != want {
		t.Errorf("TargetBuildDir(%q) = %q, want %q", "device", got, want)
	}
}

func TestProject_ResolveLayerPath_Relative(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}
	got := p.ResolveLayerPath("layers/base")
	want := "/home/user/Edge-OS/layers/base"
	if got != want {
		t.Errorf("ResolveLayerPath(%q) = %q, want %q", "layers/base", got, want)
	}
}

func TestProject_ResolveLayerPath_Absolute(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}
	got := p.ResolveLayerPath("/opt/shared/layer")
	want := "/opt/shared/layer"
	if got != want {
		t.Errorf("ResolveLayerPath(%q) = %q, want %q", "/opt/shared/layer", got, want)
	}
}

func TestProject_ResolveLayerPath_URL(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}
	url := "https://github.com/example/layer.git"
	got := p.ResolveLayerPath(url)
	if got != url {
		t.Errorf("ResolveLayerPath(%q) = %q, want unchanged", url, got)
	}
}

func TestLoadProject_ValidEdgeOS(t *testing.T) {
	// Create a project YAML similar to Edge-OS starforge.yaml
	dir := t.TempDir()
	content := `name: Edge-OS
description: TelemetryOS Edge Device Image
targets:
  device:
    args:
      device_hostname: edge-device
    layers:
      - layers/base
      - layers/graphical
      - layers/player
  device-dev:
    args:
      device_hostname: edge-dev
    layers:
      - layers/base
      - layers/graphical
      - layers/player
      - layers/development
  installer:
    layers:
      - layers/installer-base
      - layers/installer
    qemu:
      memory: 4096
      cpus: 4
`
	os.WriteFile(filepath.Join(dir, "starforge.yaml"), []byte(content), 0o644)

	proj, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject error: %v", err)
	}

	if proj.Name != "Edge-OS" {
		t.Errorf("Name = %q, want %q", proj.Name, "Edge-OS")
	}
	if proj.Description != "TelemetryOS Edge Device Image" {
		t.Errorf("Description = %q", proj.Description)
	}
	if len(proj.Targets) != 3 {
		t.Errorf("len(Targets) = %d, want 3", len(proj.Targets))
	}

	// Check device target
	device := proj.Targets["device"]
	if len(device.Layers) != 3 {
		t.Errorf("device layers = %d, want 3", len(device.Layers))
	}
	if device.Args["device_hostname"] != "edge-device" {
		t.Errorf("device_hostname = %q", device.Args["device_hostname"])
	}

	// Check installer target has QEMU config
	installer := proj.Targets["installer"]
	if installer.QEMU == nil {
		t.Fatal("installer QEMU config is nil")
	}
	if installer.QEMU.Memory != 4096 {
		t.Errorf("QEMU memory = %d, want 4096", installer.QEMU.Memory)
	}
	if installer.QEMU.CPUs != 4 {
		t.Errorf("QEMU cpus = %d, want 4", installer.QEMU.CPUs)
	}

	// Dir should be set
	if proj.Dir == "" {
		t.Error("Dir should be set after LoadProject")
	}
}

func TestLoadProject_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadProject(dir)
	if err == nil {
		t.Fatal("expected error for missing starforge.yaml")
	}
}

func TestLoadProject_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "starforge.yaml"), []byte(":::invalid"), 0o644)
	_, err := LoadProject(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadProject_FailsValidation(t *testing.T) {
	dir := t.TempDir()
	// Valid YAML but fails validation (no name)
	os.WriteFile(filepath.Join(dir, "starforge.yaml"), []byte("targets:\n  x:\n    layers: [a]"), 0o644)
	_, err := LoadProject(dir)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadProject_WithEnvAndQEMU(t *testing.T) {
	dir := t.TempDir()
	content := `name: test
targets:
  run:
    env:
      DISPLAY: ":0"
    layers:
      - base
    qemu:
      memory: 2048
      cpus: 2
      gpu_memory: 256
      display: "gtk,gl=on"
      cpu: "host"
      ssh_port: 2222
      disks:
        - name: data
          size: 10G
      args:
        - "-enable-kvm"
`
	os.WriteFile(filepath.Join(dir, "starforge.yaml"), []byte(content), 0o644)

	proj, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject error: %v", err)
	}

	target := proj.Targets["run"]
	if target.Env["DISPLAY"] != ":0" {
		t.Errorf("env DISPLAY = %q", target.Env["DISPLAY"])
	}
	q := target.QEMU
	if q == nil {
		t.Fatal("QEMU config is nil")
	}
	if q.Memory != 2048 {
		t.Errorf("Memory = %d", q.Memory)
	}
	if q.GPUMemory != 256 {
		t.Errorf("GPUMemory = %d", q.GPUMemory)
	}
	if q.Display != "gtk,gl=on" {
		t.Errorf("Display = %q", q.Display)
	}
	if q.SSHPort != 2222 {
		t.Errorf("SSHPort = %d", q.SSHPort)
	}
	if len(q.Disks) != 1 || q.Disks[0].Name != "data" {
		t.Errorf("Disks = %+v", q.Disks)
	}
	if len(q.Args) != 1 || q.Args[0] != "-enable-kvm" {
		t.Errorf("Args = %v", q.Args)
	}
}

func TestProject_TargetBuildDir_SafeName(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}
	got := p.TargetBuildDir("device")
	want := "/home/user/Edge-OS/.starforge/device"
	if got != want {
		t.Errorf("TargetBuildDir(%q) = %q, want %q", "device", got, want)
	}
}

func TestProject_TargetBuildDir_PathTraversal(t *testing.T) {
	p := &Project{Dir: "/home/user/Edge-OS"}

	// A target name containing path traversal must be sanitized to its base name.
	traversal := "../../etc/passwd"
	got := p.TargetBuildDir(traversal)
	// Must resolve inside .starforge/, not escape to /etc/passwd
	buildDir := p.BuildDir()
	if !filepath.IsAbs(got) {
		t.Errorf("TargetBuildDir(%q) = %q: expected absolute path", traversal, got)
	}
	if got == filepath.Join("/", "etc", "passwd") {
		t.Errorf("TargetBuildDir(%q) escaped the build directory: %q", traversal, got)
	}
	if len(got) <= len(buildDir) {
		t.Errorf("TargetBuildDir(%q) = %q: expected path inside %q", traversal, got, buildDir)
	}
}

func TestProject_TargetBuildDir_DotDotName(t *testing.T) {
	p := &Project{Dir: "/build/project"}
	got := p.TargetBuildDir("..")
	// ".." should be replaced with the safe placeholder "_", never escape buildDir
	buildDir := p.BuildDir()
	if got == filepath.Dir(buildDir) {
		t.Errorf("TargetBuildDir(%q) escaped to parent: %q", "..", got)
	}
	// Expect the placeholder
	want := filepath.Join(buildDir, "_")
	if got != want {
		t.Errorf("TargetBuildDir(%q) = %q, want %q", "..", got, want)
	}
}
