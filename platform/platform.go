// Package platform defines the immutable target platform built by StarForge.
package platform

import (
	"fmt"
	"runtime"
)

// Architecture identifies the target CPU instruction set.
type Architecture string

const (
	// ArchitectureX86_64 is the 64-bit x86 architecture used by Arch Linux.
	ArchitectureX86_64 Architecture = "x86_64"
	// ArchitectureAArch64 is the 64-bit ARM architecture.
	ArchitectureAArch64 Architecture = "aarch64"
)

// Machine identifies the boot and virtual hardware contract for an image.
type Machine string

const (
	// MachineGenericUEFI is a generic UEFI virtual or physical machine.
	MachineGenericUEFI Machine = "generic-uefi"
)

// Distribution identifies the package distribution used to build the target.
type Distribution string

const (
	// DistributionArchLinux is the official x86-64 Arch Linux distribution.
	DistributionArchLinux Distribution = "archlinux"
	// DistributionArchLinuxARM is the Arch Linux ARM distribution.
	DistributionArchLinuxARM Distribution = "archlinuxarm"
	// DistributionArchLinuxPorts is the Arch Linux Ports distribution.
	DistributionArchLinuxPorts Distribution = "archlinux-ports"
)

// Spec describes the target platform independently from where the build runs.
type Spec struct {
	Architecture Architecture `yaml:"architecture,omitempty" json:"architecture"`
	Machine      Machine      `yaml:"machine,omitempty" json:"machine"`
	Distribution Distribution `yaml:"distribution,omitempty" json:"distribution"`
}

// Resolve applies deterministic defaults to an incomplete platform spec.
func (s Spec) Resolve() Spec {
	if s.Architecture == "" {
		s.Architecture = ArchitectureX86_64
	}
	if s.Machine == "" {
		s.Machine = MachineGenericUEFI
	}
	if s.Distribution == "" {
		switch s.Architecture {
		case ArchitectureAArch64:
			s.Distribution = DistributionArchLinuxARM
		default:
			s.Distribution = DistributionArchLinux
		}
	}
	return s
}

// Validate checks whether the resolved platform is supported by StarForge.
func (s Spec) Validate() error {
	s = s.Resolve()

	switch s.Architecture {
	case ArchitectureX86_64, ArchitectureAArch64:
	default:
		return fmt.Errorf("unsupported architecture %q", s.Architecture)
	}

	if s.Machine != MachineGenericUEFI {
		return fmt.Errorf("unsupported machine %q", s.Machine)
	}

	switch s.Distribution {
	case DistributionArchLinux:
		if s.Architecture != ArchitectureX86_64 {
			return fmt.Errorf("distribution %q only supports architecture %q", s.Distribution, ArchitectureX86_64)
		}
	case DistributionArchLinuxARM, DistributionArchLinuxPorts:
		if s.Architecture != ArchitectureAArch64 {
			return fmt.Errorf("distribution %q only supports architecture %q", s.Distribution, ArchitectureAArch64)
		}
	default:
		return fmt.Errorf("unsupported distribution %q", s.Distribution)
	}

	return nil
}

// GoArch returns the GOARCH value for the target architecture.
func (s Spec) GoArch() string {
	switch s.Resolve().Architecture {
	case ArchitectureAArch64:
		return "arm64"
	default:
		return "amd64"
	}
}

// HostArchitecture returns the StarForge architecture of the running binary.
func HostArchitecture() (Architecture, error) {
	switch runtime.GOARCH {
	case "amd64":
		return ArchitectureX86_64, nil
	case "arm64":
		return ArchitectureAArch64, nil
	default:
		return "", fmt.Errorf("unsupported host architecture %q", runtime.GOARCH)
	}
}

// Identity returns a stable target-platform identifier for logs and caches.
func (s Spec) Identity() string {
	r := s.Resolve()
	return fmt.Sprintf("%s/%s/%s", r.Distribution, r.Architecture, r.Machine)
}

// SystemdBootEFILoader returns the architecture-specific EFI loader path.
func (s Spec) SystemdBootEFILoader() string {
	switch s.Resolve().Architecture {
	case ArchitectureAArch64:
		return `\EFI\systemd\systemd-bootaa64.efi`
	default:
		return `\EFI\systemd\systemd-bootx64.efi`
	}
}
