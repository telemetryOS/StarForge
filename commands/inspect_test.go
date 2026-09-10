package commands

import (
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

func TestRenderUsersIncludesPinnedIdentity(t *testing.T) {
	previousInspectLayers := inspectLayers
	inspectLayers = false
	defer func() { inspectLayers = previousInspectLayers }()

	ctx := actions.NewBuildContext()
	ctx.Users = []actions.UserDef{{
		Name:         "player",
		PrimaryGroup: "player",
		UID:          1000,
	}}

	var output strings.Builder
	renderUsers(&output, ctx)
	for _, want := range []string{"player", "uid: 1000", "primary group: player"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("rendered users output does not contain %q: %q", want, output.String())
		}
	}
}

func TestRenderBootIncludesExplicitFiles(t *testing.T) {
	ctx := actions.NewBuildContext()
	ctx.Boot = &actions.BootConfig{Entries: []config.BootEntry{{
		Name:       "arm64.conf",
		Title:      "ARM64",
		Kernel:     "linux-aarch64",
		KernelFile: "Image",
		InitrdFile: "initramfs-linux.img",
	}}}

	var output strings.Builder
	renderBoot(&output, ctx)
	for _, want := range []string{"kernel file: Image", "initrd file: initramfs-linux.img"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("rendered boot output does not contain %q: %q", want, output.String())
		}
	}
}
