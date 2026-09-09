// Package executor runs StarForge builds in a selected execution environment.
package executor

import (
	"context"
	"fmt"
	"os"

	"github.com/telemetryos/starforge/appliance"
	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/platform"
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
	case ModeAuto:
		return &Auto{}, nil
	case ModeNative:
		return &Native{}, nil
	case ModeVM:
		manager, err := appliance.NewManager()
		if err != nil {
			return nil, err
		}
		return &VM{manager: manager}, nil
	default:
		return nil, fmt.Errorf("unsupported executor mode %q", mode)
	}
}

// Auto selects native execution only for an already-root same-architecture
// process. All other builds use the unprivileged VM executor.
type Auto struct{}

// Mode returns the automatic executor mode.
func (*Auto) Mode() Mode { return ModeAuto }

// Prepare defers environment selection until the target request is available.
func (*Auto) Prepare(context.Context) error { return nil }

// Build selects and invokes an executor for the request.
func (*Auto) Build(ctx context.Context, request BuildRequest) (*BuildResult, error) {
	selected, err := autoExecutor(request)
	if err != nil {
		return nil, err
	}
	if err := selected.Prepare(ctx); err != nil {
		return nil, err
	}
	return selected.Build(ctx, request)
}

// EnsureArtifact selects an executor and creates a sealed disk artifact.
func (*Auto) EnsureArtifact(ctx context.Context, request BuildRequest) (*ArtifactResult, error) {
	selected, err := autoExecutor(request)
	if err != nil {
		return nil, err
	}
	if err := selected.Prepare(ctx); err != nil {
		return nil, err
	}
	return selected.EnsureArtifact(ctx, request)
}

func autoExecutor(request BuildRequest) (BuildExecutor, error) {
	host, err := platform.HostArchitecture()
	if err != nil {
		return nil, err
	}
	if os.Geteuid() == 0 && host == request.Target.ResolvedPlatform().Architecture {
		return &Native{}, nil
	}
	manager, err := appliance.NewManager()
	if err != nil {
		return nil, err
	}
	return &VM{manager: manager}, nil
}
