// Package distribution defines target package repository and trust backends.
package distribution

import (
	"fmt"

	"github.com/telemetryos/starforge/platform"
)

// Repository is an ordered pacman repository definition.
type Repository struct {
	Name   string
	Server string
}

// PublicKey is an explicitly trusted package-signing key.
type PublicKey struct {
	URL         string
	Fingerprint string
}

// Archive describes a package archive used for version-pinned packages.
type Archive struct {
	BaseURL       string
	Architectures []string
}

// Backend supplies package repositories, signing trust, and installed config.
type Backend interface {
	Distribution() platform.Distribution
	Repositories() []Repository
	PopulateKeyrings() []string
	PublicKeys() []PublicKey
	RequiredPackages() []string
	Archive() *Archive
}

// ForPlatform resolves the package backend for a validated target platform.
func ForPlatform(target platform.Spec) (Backend, error) {
	target = target.Resolve()
	if err := target.Validate(); err != nil {
		return nil, err
	}
	switch target.Distribution {
	case platform.DistributionArchLinux:
		return archLinux{}, nil
	case platform.DistributionArchLinuxARM:
		return archLinuxARM{}, nil
	case platform.DistributionArchLinuxPorts:
		return archLinuxPorts{}, nil
	default:
		return nil, fmt.Errorf("unsupported distribution %q", target.Distribution)
	}
}

type archLinux struct{}

func (archLinux) Distribution() platform.Distribution { return platform.DistributionArchLinux }
func (archLinux) Repositories() []Repository {
	return []Repository{
		{Name: "core", Server: "https://geo.mirror.pkgbuild.com/$repo/os/$arch"},
		{Name: "extra", Server: "https://geo.mirror.pkgbuild.com/$repo/os/$arch"},
	}
}
func (archLinux) PopulateKeyrings() []string { return []string{"archlinux"} }
func (archLinux) PublicKeys() []PublicKey    { return nil }
func (archLinux) RequiredPackages() []string { return []string{"archlinux-keyring"} }
func (archLinux) Archive() *Archive {
	return &Archive{
		BaseURL:       "https://archive.archlinux.org/packages",
		Architectures: []string{"x86_64", "any"},
	}
}

type archLinuxARM struct{}

func (archLinuxARM) Distribution() platform.Distribution { return platform.DistributionArchLinuxARM }
func (archLinuxARM) Repositories() []Repository {
	return []Repository{
		{Name: "core", Server: "https://ca.us.mirror.archlinuxarm.org/$arch/$repo"},
		{Name: "extra", Server: "https://ca.us.mirror.archlinuxarm.org/$arch/$repo"},
		{Name: "alarm", Server: "https://ca.us.mirror.archlinuxarm.org/$arch/$repo"},
		{Name: "aur", Server: "https://ca.us.mirror.archlinuxarm.org/$arch/$repo"},
	}
}
func (archLinuxARM) PopulateKeyrings() []string { return []string{"archlinuxarm"} }
func (archLinuxARM) PublicKeys() []PublicKey    { return nil }
func (archLinuxARM) RequiredPackages() []string { return []string{"archlinuxarm-keyring"} }
func (archLinuxARM) Archive() *Archive          { return nil }

type archLinuxPorts struct{}

func (archLinuxPorts) Distribution() platform.Distribution {
	return platform.DistributionArchLinuxPorts
}
func (archLinuxPorts) Repositories() []Repository {
	return []Repository{
		{Name: "core", Server: "https://arch-linux-repo.drzee.net/arch/$repo/os/$arch"},
		{Name: "extra", Server: "https://arch-linux-repo.drzee.net/arch/$repo/os/$arch"},
		{Name: "forge", Server: "https://arch-linux-repo.drzee.net/arch/$repo/os/$arch"},
	}
}
func (archLinuxPorts) PopulateKeyrings() []string { return nil }
func (archLinuxPorts) PublicKeys() []PublicKey {
	return []PublicKey{{
		URL:         "https://arch-linux-repo.drzee.net/arch/extra/os/aarch64/public.key",
		Fingerprint: "9B2C213B21883BB65CE2FB900CF25682E6BA0751",
	}}
}
func (archLinuxPorts) RequiredPackages() []string { return nil }
func (archLinuxPorts) Archive() *Archive          { return nil }
