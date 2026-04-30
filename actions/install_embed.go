package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

// InstallEmbed records that another target's full build should be merged into
// this target's disk image at packaging time. The embedded target builds
// independently — its own rootfs overlay, its own actions — and its partition
// declarations are unioned with this target's by name. See MergePartitions
// for the merge rules.
//
// Pairs with install-payload, which depends on a target's packaged images.
// Both are forms of "this target depends on another target."
type InstallEmbed struct{}

func (a *InstallEmbed) Name() string { return "install-embed" }

func (a *InstallEmbed) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.InstallEmbed
	if s == nil {
		return fmt.Errorf("install-embed: step is nil")
	}
	if s.Target == "" {
		return fmt.Errorf("install-embed: target is required")
	}
	for _, existing := range ctx.InstallEmbeds {
		if existing == s.Target {
			// Idempotent: declaring the same embed twice is harmless.
			return nil
		}
	}
	ctx.InstallEmbeds = append(ctx.InstallEmbeds, s.Target)
	return nil
}
