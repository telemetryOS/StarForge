package executor

import (
	"context"
	"os"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/platform"
)

func TestVMEnsureArtifactReusesSealedArtifact(t *testing.T) {
	targetPlatform := platform.Spec{
		Architecture: platform.ArchitectureAArch64,
		Machine:      platform.MachineGenericUEFI,
		Distribution: platform.DistributionArchLinuxPorts,
	}
	project := &config.Project{Dir: t.TempDir()}
	target := config.Target{Platform: targetPlatform}
	store := artifact.NewStore(project.TargetBuildDir("device"), "device", targetPlatform)
	receipt, err := store.Begin("source-hash")
	if err != nil {
		t.Fatal(err)
	}
	receipt.Partitions = []actions.PartitionDef{{Name: "root", Size: 1}}
	if err := os.WriteFile(store.CreatingDiskPath(), []byte("sealed disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err = store.Seal(receipt)
	if err != nil {
		t.Fatal(err)
	}

	result, err := (&VM{}).EnsureArtifact(context.Background(), BuildRequest{
		Project:    project,
		TargetName: "device",
		Target:     target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.SHA256 != receipt.SHA256 || result.Disk != store.DiskPath() {
		t.Fatalf("reused artifact = %#v, want receipt %s at %s", result, receipt.SHA256, store.DiskPath())
	}
	if len(result.Build.Result.Partitions) != 1 || result.Build.Result.Partitions[0].Name != "root" {
		t.Fatalf("reused build result partitions = %#v", result.Build.Result.Partitions)
	}
}
