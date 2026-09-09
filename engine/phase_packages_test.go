package engine

import (
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/distribution"
	"github.com/telemetryos/starforge/platform"
)

func TestRenderPacmanConfUsesTargetArchitectureAndRepositories(t *testing.T) {
	target := platform.Spec{Architecture: platform.ArchitectureAArch64}.Resolve()
	backend, err := distribution.ForPlatform(target)
	if err != nil {
		t.Fatal(err)
	}
	conf := renderPacmanConf("/cache", "/gpg", target, backend)
	for _, expected := range []string{
		"Architecture = aarch64",
		"[core]",
		"[extra]",
		"[alarm]",
		"[aur]",
		"mirror.archlinuxarm.org/$arch/$repo",
	} {
		if !strings.Contains(conf, expected) {
			t.Errorf("pacman config missing %q:\n%s", expected, conf)
		}
	}
	if strings.Contains(conf, "geo.mirror.pkgbuild.com") {
		t.Fatalf("x86 Arch mirror leaked into ARM config:\n%s", conf)
	}
}

func TestRenderPacmanConfUsesPortsForgeRepository(t *testing.T) {
	target := platform.Spec{
		Architecture: platform.ArchitectureAArch64,
		Distribution: platform.DistributionArchLinuxPorts,
	}.Resolve()
	backend, err := distribution.ForPlatform(target)
	if err != nil {
		t.Fatal(err)
	}
	conf := renderPacmanConf("/cache", "/gpg", target, backend)
	if !strings.Contains(conf, "[forge]") || !strings.Contains(conf, "arch-linux-repo.drzee.net") {
		t.Fatalf("ports repositories missing:\n%s", conf)
	}
}

func TestResolvePkgRelsRejectsPinnedARMWithoutArchive(t *testing.T) {
	builder := NewBuilder(nil)
	ctx := actions.NewBuildContext()
	ctx.Platform = platform.Spec{Architecture: platform.ArchitectureAArch64}.Resolve()
	ctx.Packages = []actions.Package{{Name: "bash", Version: "5.3"}}
	if err := builder.resolvePkgRels(ctx, false); err == nil {
		t.Fatal("pinned Arch Linux ARM package unexpectedly resolved without an archive")
	}
}
