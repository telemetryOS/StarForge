package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Project represents a starforge.yaml project configuration.
type Project struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Targets     map[string]Target `yaml:"targets"`

	// Dir is the absolute path to the project directory (not serialized).
	Dir string `yaml:"-"`
}

// Target defines a named build profile with an ordered list of layers.
type Target struct {
	Args       map[string]string `yaml:"args,omitempty"`
	DefaultEnv map[string]string `yaml:"default_env,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	Layers     []string          `yaml:"layers"`
	QEMU       *QEMUConfig       `yaml:"qemu,omitempty"`
}

// QEMUConfig holds QEMU virtual machine configuration for a target.
type QEMUConfig struct {
	Memory    int        `yaml:"memory,omitempty"`
	CPUs      int        `yaml:"cpus,omitempty"`
	GPUMemory int        `yaml:"gpu_memory,omitempty"`
	Display   string     `yaml:"display,omitempty"`
	CPU       string     `yaml:"cpu,omitempty"`
	Audio     string     `yaml:"audio,omitempty"`
	SSHPort   int        `yaml:"ssh_port,omitempty"`
	Disks     []QEMUDisk `yaml:"disks,omitempty"`
	Args      []string   `yaml:"args,omitempty"`
}

// QEMUDisk defines an additional virtual disk for QEMU.
type QEMUDisk struct {
	Name string `yaml:"name"`
	Size string `yaml:"size"`
}

const ProjectFile = "starforge.yaml"

// LoadProject reads and parses a starforge.yaml from the given directory.
func LoadProject(dir string) (*Project, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving project path: %w", err)
	}

	path := filepath.Join(absDir, ProjectFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ProjectFile, err)
	}

	var proj Project
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ProjectFile, err)
	}

	proj.Dir = absDir

	if err := proj.Validate(); err != nil {
		return nil, err
	}

	return &proj, nil
}

// FindProject walks up from the current directory to find a starforge.yaml.
func FindProject() (*Project, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	for {
		path := filepath.Join(dir, ProjectFile)
		if _, err := os.Stat(path); err == nil {
			return LoadProject(dir)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no %s found in current directory or any parent", ProjectFile)
		}
		dir = parent
	}
}

// Validate checks the project configuration for errors.
func (p *Project) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if len(p.Targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}
	for name, target := range p.Targets {
		if len(target.Layers) == 0 {
			return fmt.Errorf("target %q has no layers", name)
		}
	}
	return nil
}

// BuildDir returns the path to the .starforge build artifacts directory.
func (p *Project) BuildDir() string {
	return filepath.Join(p.Dir, ".starforge")
}

// TargetBuildDir returns the build directory for a specific target.
// Target names must be simple identifiers (no path separators or ".." components)
// to prevent escaping the build directory.
func (p *Project) TargetBuildDir(target string) string {
	// Sanitize the target name to a single safe path component so that a
	// name like "../../etc" cannot escape the build directory.
	// Use only the final component of the cleaned path, and replace unsafe
	// base names (".", "..") with a safe placeholder.
	clean := filepath.Base(filepath.Clean(target))
	if clean == "." || clean == ".." || clean == "" {
		clean = "_"
	}
	return filepath.Join(p.BuildDir(), clean)
}

// ResolveLayerPath resolves a layer path relative to the project directory.
// URL paths are returned as-is.
func (p *Project) ResolveLayerPath(layerPath string) string {
	if IsURL(layerPath) {
		return layerPath
	}
	if filepath.IsAbs(layerPath) {
		return layerPath
	}
	return filepath.Join(p.Dir, layerPath)
}
