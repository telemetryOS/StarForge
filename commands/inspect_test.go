package commands

import (
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
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
