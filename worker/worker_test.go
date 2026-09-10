package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/platform"
)

func TestLoadJobRequiresMatchingProtocol(t *testing.T) {
	job := Job{
		Target: "device",
		Platform: platform.Spec{
			Architecture: platform.ArchitectureX86_64,
			Machine:      platform.MachineGenericUEFI,
			Distribution: platform.DistributionArchLinux,
		},
	}
	path := writeJob(t, job)
	if _, err := loadJob(path); err == nil || !strings.Contains(err.Error(), "worker protocol") {
		t.Fatalf("loadJob error = %v", err)
	}

	job.Protocol = ProtocolVersion
	path = writeJob(t, job)
	if _, err := loadJob(path); err != nil {
		t.Fatalf("loadJob with current protocol: %v", err)
	}
}

func writeJob(t *testing.T, job Job) string {
	t.Helper()
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "job.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
