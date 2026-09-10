package executor

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/engine"
	"github.com/telemetryos/starforge/platform"
)

// Native executes the existing privileged StarForge engine on the host.
type Native struct{}

// Mode returns the executor's explicit mode.
func (*Native) Mode() Mode {
	return ModeNative
}

// Prepare elevates the process before host build resources are initialized.
func (*Native) Prepare(context.Context) error {
	if err := engine.EnsureRootExec(); err != nil {
		return fmt.Errorf("failed to elevate privileges: %w", err)
	}
	return nil
}

// Build runs the existing layer and overlay build engine on the host.
func (*Native) Build(ctx context.Context, request BuildRequest) (*BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateNativeArchitecture(request); err != nil {
		return nil, err
	}
	builder, err := newNativeBuilder(request)
	if err != nil {
		return nil, err
	}
	if err := builder.Build(request.TargetName, request.Target, request.Clean); err != nil {
		return nil, err
	}
	buildDir := request.Project.TargetBuildDir(request.TargetName)
	result, err := engine.LoadBuildResult(buildDir)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	engine.ChownToInvoker(request.Project.BuildDir())
	return &BuildResult{BuildDir: buildDir, Result: result}, nil
}

// EnsureArtifact builds, packages, and seals a complete raw disk image.
func (*Native) EnsureArtifact(ctx context.Context, request BuildRequest) (*ArtifactResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateNativeArchitecture(request); err != nil {
		return nil, err
	}

	builder, err := newNativeBuilder(request)
	if err != nil {
		return nil, err
	}
	var buildContextErr error
	var buildContext *actions.BuildContext
	if request.Clean {
		if err := builder.Build(request.TargetName, request.Target, true); err != nil {
			return nil, err
		}
		buildContext, buildContextErr = builder.EnsurePackaged(request.TargetName)
	} else {
		buildContext, buildContextErr = builder.EnsureBuiltAndPackaged(request.TargetName)
	}
	if buildContextErr != nil {
		return nil, buildContextErr
	}

	buildDir := request.Project.TargetBuildDir(request.TargetName)
	buildResult, err := engine.LoadBuildResult(buildDir)
	if err != nil {
		return nil, err
	}
	result := &BuildResult{BuildDir: buildDir, Result: buildResult}

	manifest, err := engine.LoadManifest(filepath.Join(buildDir, "cache"))
	if err != nil {
		return nil, err
	}
	if manifest.Packaging == nil || !manifest.Packaging.Completed {
		return nil, fmt.Errorf("target %q packaging is incomplete", request.TargetName)
	}

	store := artifact.NewStore(buildDir, request.TargetName, buildContext.Platform)
	if receipt, loadErr := store.Load(); loadErr == nil && receipt.State == artifact.StateSealed && receipt.SourceHash == manifest.Packaging.Hash {
		return &ArtifactResult{Build: result, Receipt: receipt, Disk: store.DiskPath()}, nil
	}

	receipt, err := store.Begin(manifest.Packaging.Hash)
	if err != nil {
		return nil, err
	}
	receipt.Partitions = append([]actions.PartitionDef(nil), buildContext.Partitions...)
	abort := func(buildErr error) error {
		if receiptErr := store.Abort(receipt, buildErr); receiptErr != nil {
			return fmt.Errorf("%w (recording artifact failure: %v)", buildErr, receiptErr)
		}
		return buildErr
	}

	loopDevice, cleanup, err := engine.WriteToDiskImage(buildContext.Partitions, buildDir, store.CreatingDiskPath())
	if err != nil {
		return nil, abort(err)
	}
	if engine.HasInstallerActions(buildContext) {
		if err := builder.BundleInstallerToDevice(buildContext, loopDevice); err != nil {
			cleanup()
			return nil, abort(err)
		}
	}
	cleanup()

	if err := ctx.Err(); err != nil {
		return nil, abort(err)
	}
	receipt, err = store.Seal(receipt)
	if err != nil {
		return nil, abort(err)
	}
	engine.ChownToInvoker(request.Project.BuildDir())
	return &ArtifactResult{Build: result, Receipt: receipt, Disk: store.DiskPath()}, nil
}

func validateNativeArchitecture(request BuildRequest) error {
	hostArchitecture, err := platform.HostArchitecture()
	if err != nil {
		return err
	}
	targetArchitecture := request.Target.ResolvedPlatform().Architecture
	if targetArchitecture != hostArchitecture {
		return fmt.Errorf("native executor cannot build %s on %s; use --executor=vm", targetArchitecture, hostArchitecture)
	}
	return nil
}

func newNativeBuilder(request BuildRequest) (*engine.Builder, error) {
	hostArchitecture, err := platform.HostArchitecture()
	if err != nil {
		return nil, err
	}
	if hostArchitecture == platform.ArchitectureAArch64 {
		return engine.NewBuilderWithHostTools(request.Project, engine.SystemHostTools{}), nil
	}
	return engine.NewBuilder(request.Project), nil
}
