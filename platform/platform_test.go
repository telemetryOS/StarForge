package platform

import "testing"

func TestSpecResolveDefaultsToArchLinuxX8664(t *testing.T) {
	got := (Spec{}).Resolve()
	want := Spec{
		Architecture: ArchitectureX86_64,
		Machine:      MachineGenericUEFI,
		Distribution: DistributionArchLinux,
	}
	if got != want {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
}

func TestSpecResolveDefaultsAArch64Distribution(t *testing.T) {
	got := (Spec{Architecture: ArchitectureAArch64}).Resolve()
	if got.Distribution != DistributionArchLinuxARM {
		t.Fatalf("Distribution = %q, want %q", got.Distribution, DistributionArchLinuxARM)
	}
	if got.GoArch() != "arm64" {
		t.Fatalf("GoArch() = %q, want arm64", got.GoArch())
	}
}

func TestSpecValidateSupportedCombinations(t *testing.T) {
	tests := []Spec{
		{},
		{Architecture: ArchitectureX86_64, Distribution: DistributionArchLinux},
		{Architecture: ArchitectureAArch64, Distribution: DistributionArchLinuxARM},
		{Architecture: ArchitectureAArch64, Distribution: DistributionArchLinuxPorts},
	}
	for _, spec := range tests {
		if err := spec.Validate(); err != nil {
			t.Errorf("Validate(%#v) error: %v", spec, err)
		}
	}
}

func TestSpecValidateRejectsUnsupportedCombinations(t *testing.T) {
	tests := []Spec{
		{Architecture: "riscv64"},
		{Machine: "raspberry-pi-5"},
		{Architecture: ArchitectureAArch64, Distribution: DistributionArchLinux},
		{Architecture: ArchitectureX86_64, Distribution: DistributionArchLinuxARM},
		{Distribution: "unknown"},
	}
	for _, spec := range tests {
		if err := spec.Validate(); err == nil {
			t.Errorf("Validate(%#v) unexpectedly succeeded", spec)
		}
	}
}

func TestSpecIdentity(t *testing.T) {
	got := (Spec{Architecture: ArchitectureAArch64}).Identity()
	want := "archlinuxarm/aarch64/generic-uefi"
	if got != want {
		t.Fatalf("Identity() = %q, want %q", got, want)
	}
}

func TestSpecSystemdBootEFILoader(t *testing.T) {
	tests := []struct {
		spec Spec
		want string
	}{
		{spec: Spec{}, want: `\EFI\systemd\systemd-bootx64.efi`},
		{spec: Spec{Architecture: ArchitectureAArch64}, want: `\EFI\systemd\systemd-bootaa64.efi`},
	}
	for _, test := range tests {
		if got := test.spec.SystemdBootEFILoader(); got != test.want {
			t.Errorf("SystemdBootEFILoader(%#v) = %q, want %q", test.spec, got, test.want)
		}
	}
}
