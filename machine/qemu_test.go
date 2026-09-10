package machine

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/platform"
)

func TestBuildLaunchSpecSelectsArchitecture(t *testing.T) {
	disk := createTestDisk(t)
	tests := []struct {
		architecture platform.Architecture
		binary       string
		machineArg   string
	}{
		{platform.ArchitectureX86_64, "qemu-system-x86_64", "q35"},
		{platform.ArchitectureAArch64, "qemu-system-aarch64", "virt"},
	}
	for _, test := range tests {
		receipt := &artifact.Receipt{
			State:    artifact.StateSealed,
			Platform: platform.Spec{Architecture: test.architecture}.Resolve(),
		}
		spec, err := buildLaunchSpec(RunRequest{Artifact: receipt, Disk: disk, Serial: true}, testQEMUHost(false))
		if err != nil {
			t.Fatalf("BuildLaunchSpec(%s): %v", test.architecture, err)
		}
		if spec.Binary != test.binary {
			t.Errorf("binary = %q, want %q", spec.Binary, test.binary)
		}
		if !slices.Contains(spec.Args, test.machineArg) {
			t.Errorf("args do not contain machine %q: %v", test.machineArg, spec.Args)
		}
		joined := strings.Join(spec.Args, " ")
		if !strings.Contains(joined, "snapshot=on") {
			t.Errorf("sealed base disk is writable: %s", joined)
		}
	}
}

func TestBuildLaunchSpecUsesTCGWhenKVMUnavailable(t *testing.T) {
	disk := createTestDisk(t)
	receipt := &artifact.Receipt{State: artifact.StateSealed, Platform: platform.Spec{Architecture: platform.ArchitectureAArch64}.Resolve()}
	spec, err := buildLaunchSpec(RunRequest{Artifact: receipt, Disk: disk, Serial: true}, testQEMUHost(false))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(spec.Args, " ")
	if !strings.Contains(joined, "-accel tcg") || strings.Contains(joined, "-enable-kvm") {
		t.Fatalf("cross-architecture args do not force TCG: %s", joined)
	}
}

func TestBuildLaunchSpecConfiguresArchitectureSpecificGraphicalInput(t *testing.T) {
	disk := createTestDisk(t)
	tests := []struct {
		name         string
		architecture platform.Architecture
		want         string
		reject       string
	}{
		{name: "x86_64", architecture: platform.ArchitectureX86_64, want: "-usb -device usb-tablet", reject: "qemu-xhci"},
		{name: "aarch64", architecture: platform.ArchitectureAArch64, want: "-device qemu-xhci,id=xhci -device usb-tablet,bus=xhci.0", reject: "-usb -device usb-tablet"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := &artifact.Receipt{
				State:    artifact.StateSealed,
				Platform: platform.Spec{Architecture: test.architecture}.Resolve(),
			}
			spec, err := buildLaunchSpec(RunRequest{Artifact: receipt, Disk: disk}, testQEMUHost(false))
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(spec.Args, " ")
			if !strings.Contains(joined, test.want) {
				t.Fatalf("graphical args do not contain %q: %s", test.want, joined)
			}
			if strings.Contains(joined, test.reject) {
				t.Fatalf("graphical args unexpectedly contain %q: %s", test.reject, joined)
			}
		})
	}
}

func testQEMUHost(kvm bool) qemuHost {
	return qemuHost{
		lookPath: func(binary string) (string, error) {
			return "/usr/bin/" + binary, nil
		},
		findFirmware: func(spec platform.Spec) (string, error) {
			if spec.Architecture == "" {
				return "", fmt.Errorf("missing architecture")
			}
			return "/firmware.fd", nil
		},
		kvmAvailable: func(platform.Spec) bool {
			return kvm
		},
	}
}

func createTestDisk(t *testing.T) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "disk-*.raw")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}
