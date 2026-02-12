package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type InstallServer struct{}

func (a *InstallServer) Name() string { return "install-server" }

// installerDeps are packages required at install time for partitioning,
// formatting, and payload decompression.
var installerDeps = []string{
	"dosfstools",           // mkfs.vfat
	"e2fsprogs",            // mkfs.ext4
	"arch-install-scripts", // genfstab, arch-chroot
	"zstd",                 // zstd decompression
}

func (a *InstallServer) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.InstallServer

	if s.Path == "" {
		return fmt.Errorf("install-server: path is required")
	}

	port := s.Port
	if port == 0 {
		port = 8100
	}

	ctx.InstallerServer = &InstallerServerDef{
		Port:  port,
		Path:  s.Path,
		Layer: ctx.CurrentLayer,
	}

	// Add installer runtime dependencies to the package list
	ctx.Packages = append(ctx.Packages, installerDeps...)
	ctx.PackageGroups = append(ctx.PackageGroups, LayerGroup{
		Layer: ctx.CurrentLayer,
		Items: installerDeps,
	})

	return nil
}
