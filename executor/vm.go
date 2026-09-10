package executor

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/appliance"
	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/engine"
)

// VM runs the privileged build engine inside a managed QEMU appliance.
type VM struct {
	manager *appliance.Manager
}

// Mode returns the explicit VM executor mode.
func (*VM) Mode() Mode { return ModeVM }

// Prepare performs no privileged host setup; target-specific checks run with the request.
func (*VM) Prepare(context.Context) error { return nil }

// Build creates the target's sealed full-disk artifact in the builder VM.
func (v *VM) Build(ctx context.Context, request BuildRequest) (*BuildResult, error) {
	result, err := v.buildArtifact(ctx, request)
	if err != nil {
		return nil, err
	}
	return result.Build, nil
}

// EnsureArtifact returns the current sealed artifact or builds one when absent.
func (v *VM) EnsureArtifact(ctx context.Context, request BuildRequest) (*ArtifactResult, error) {
	if request.Project == nil {
		return nil, fmt.Errorf("project is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !request.Clean {
		if result, ok := sealedArtifact(request); ok {
			return result, nil
		}
	}
	return v.buildArtifact(ctx, request)
}

func (v *VM) buildArtifact(ctx context.Context, request BuildRequest) (*ArtifactResult, error) {
	if request.Project == nil {
		return nil, fmt.Errorf("project is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	targetPlatform := request.Target.ResolvedPlatform()
	if err := v.manager.Prepare(targetPlatform); err != nil {
		return nil, err
	}
	partitions, err := engine.ResolveDiskLayout(request.Project, request.TargetName)
	if err != nil {
		return nil, err
	}
	diskSize, err := engine.DiskImageSize(partitions)
	if err != nil {
		return nil, err
	}
	if diskSize > math.MaxInt64 {
		return nil, fmt.Errorf("disk image size %d exceeds host file limits", diskSize)
	}

	buildDir := request.Project.TargetBuildDir(request.TargetName)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return nil, err
	}
	store := artifact.NewStore(buildDir, request.TargetName, targetPlatform)
	receipt, err := store.Begin("")
	if err != nil {
		return nil, err
	}
	receipt.Partitions = append(receipt.Partitions, partitions...)
	abort := func(buildErr error) error {
		if receiptErr := store.Abort(receipt, buildErr); receiptErr != nil {
			return fmt.Errorf("%w (recording artifact failure: %v)", buildErr, receiptErr)
		}
		return buildErr
	}
	output, err := os.OpenFile(store.CreatingDiskPath(), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err != nil {
		return nil, abort(fmt.Errorf("creating output disk: %w", err))
	}
	if err := output.Truncate(int64(diskSize)); err != nil {
		output.Close()
		return nil, abort(fmt.Errorf("sizing output disk: %w", err))
	}
	if err := output.Close(); err != nil {
		return nil, abort(err)
	}

	workerResult, err := v.manager.Run(ctx, appliance.Request{
		Project:    request.Project,
		TargetName: request.TargetName,
		Platform:   targetPlatform,
		Clean:      request.Clean,
		OutputDisk: store.CreatingDiskPath(),
	})
	if err != nil {
		return nil, abort(err)
	}
	workerResult.Build.Partitions = partitions
	if workerResult.Build.Platform.Resolve() != targetPlatform {
		return nil, abort(fmt.Errorf("worker built platform %s, expected %s", workerResult.Build.Platform.Identity(), targetPlatform.Identity()))
	}
	receipt.SourceHash = workerResult.SourceHash
	receipt, err = store.Seal(receipt)
	if err != nil {
		return nil, abort(err)
	}
	return &ArtifactResult{
		Build: &BuildResult{
			BuildDir: filepath.Clean(buildDir),
			Result:   workerResult.Build,
		},
		Receipt: receipt,
		Disk:    store.DiskPath(),
	}, nil
}

func sealedArtifact(request BuildRequest) (*ArtifactResult, bool) {
	targetPlatform := request.Target.ResolvedPlatform()
	buildDir := request.Project.TargetBuildDir(request.TargetName)
	store := artifact.NewStore(buildDir, request.TargetName, targetPlatform)
	receipt, err := store.Load()
	if err != nil || receipt.State != artifact.StateSealed {
		return nil, false
	}
	return &ArtifactResult{
		Build: &BuildResult{
			BuildDir: filepath.Clean(buildDir),
			Result: &engine.BuildResult{
				Platform:   receipt.Platform.Resolve(),
				Partitions: append([]actions.PartitionDef(nil), receipt.Partitions...),
			},
		},
		Receipt: receipt,
		Disk:    store.DiskPath(),
	}, true
}
