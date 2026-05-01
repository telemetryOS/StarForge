package actions

import (
	"github.com/telemetryos/starforge/config"
)

type InstallServer struct{}

func (a *InstallServer) Name() string { return "install-server" }

// installerDeps are packages required at install time for partitioning,
// formatting, and payload decompression.
var installerDeps = []string{
	"dosfstools",           // mkfs.vfat
	"e2fsprogs",            // mkfs.ext4
	"efibootmgr",           // EFI boot entry management
	"arch-install-scripts", // arch-chroot
	"zstd",                 // zstd decompression
	"python",               // bmaptool runtime
	"python-six",           // bmaptool runtime
}

func (a *InstallServer) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.InstallServer

	port := s.Port
	if port == 0 {
		port = 8100
	}

	path := s.Path
	if path == "" {
		path = "/usr/lib/starforge/payloads"
	}

	ctx.InstallServer = &InstallServerDef{
		Port:     port,
		Path:     path,
		Layer:    ctx.CurrentLayer,
		EFILabel: s.EFILabel,
	}

	// Add installer runtime dependencies to the package list
	for _, dep := range installerDeps {
		ctx.Packages = append(ctx.Packages, Package{Name: dep})
	}
	ctx.PackageGroups = append(ctx.PackageGroups, LayerGroup{
		Layer: ctx.CurrentLayer,
		Items: installerDeps,
	})

	return nil
}
