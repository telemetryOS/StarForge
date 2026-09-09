// Package executor runs StarForge builds in a selected execution environment.
package executor

import (
	"context"
	"fmt"

	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

// Mode identifies where privileged build operations execute.
type Mode string

const (
	// ModeAuto selects an execution environment based on host capabilities.
	ModeAuto Mode = "auto"
	// ModeNative executes build operations directly on the host.
	ModeNative Mode = "native"
	// ModeVM executes build operations in a managed QEMU appliance.
	ModeVM Mode = "vm"
)

// ParseMode validates a CLI or configuration executor mode.
func ParseMode(value string) (Mode, error) {
	mode := Mode(value)
	switch mode {
	case ModeAuto, ModeNative, ModeVM:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid executor %q: expected auto, native, or vm", value)
	}
}

// BuildRequest contains all inputs required to build one project target.
type BuildRequest struct {
	Project    *config.Project
	TargetName string
	Target     config.Target
	Clean      bool
}

// BuildResult describes a completed target build in its project workspace.
type BuildResult struct {
	BuildDir string
	Result   *engine.BuildResult
}

// ArtifactResult describes a sealed full-disk image produced by an executor.
type ArtifactResult struct {
	Build   *BuildResult
	Receipt *artifact.Receipt
	Disk    string
}

// BuildExecutor separates build orchestration from the environment that runs it.
type BuildExecutor interface {
	Mode() Mode
	Prepare(context.Context) error
	Build(context.Context, BuildRequest) (*BuildResult, error)
	EnsureArtifact(context.Context, BuildRequest) (*ArtifactResult, error)
}

// New returns the executor selected by mode.
func New(mode Mode) (BuildExecutor, error) {
	switch mode {
	case ModeAuto, ModeNative:
		return &Native{}, nil
	case ModeVM:
		return nil, fmt.Errorf("VM executor plumbing is not available yet")
	default:
		return nil, fmt.Errorf("unsupported executor mode %q", mode)
	}
}
