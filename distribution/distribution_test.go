package distribution

import (
	"testing"

	"github.com/telemetryos/starforge/platform"
)

func TestForPlatformBackends(t *testing.T) {
	tests := []struct {
		spec         platform.Spec
		distribution platform.Distribution
		repositories int
	}{
		{platform.Spec{}, platform.DistributionArchLinux, 2},
		{platform.Spec{Architecture: platform.ArchitectureAArch64}, platform.DistributionArchLinuxARM, 4},
		{platform.Spec{Architecture: platform.ArchitectureAArch64, Distribution: platform.DistributionArchLinuxPorts}, platform.DistributionArchLinuxPorts, 3},
	}
	for _, test := range tests {
		backend, err := ForPlatform(test.spec)
		if err != nil {
			t.Fatal(err)
		}
		if backend.Distribution() != test.distribution || len(backend.Repositories()) != test.repositories {
			t.Errorf("unexpected backend for %#v", test.spec)
		}
	}
}

func TestArchLinuxPortsTrustIsPinned(t *testing.T) {
	backend, err := ForPlatform(platform.Spec{
		Architecture: platform.ArchitectureAArch64,
		Distribution: platform.DistributionArchLinuxPorts,
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := backend.PublicKeys()
	if len(keys) != 1 || len(keys[0].Fingerprint) != 40 || keys[0].URL == "" {
		t.Fatalf("unexpected ports trust definition: %#v", keys)
	}
	if keyrings := backend.PopulateKeyrings(); len(keyrings) != 1 || keyrings[0] != "archlinux" {
		t.Fatalf("unexpected ports distribution keyrings: %#v", keyrings)
	}
	if packages := backend.RequiredPackages(); len(packages) != 1 || packages[0] != "archlinux-keyring" {
		t.Fatalf("unexpected ports trust packages: %#v", packages)
	}
}
