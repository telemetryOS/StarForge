package engine

import (
	"fmt"

	"github.com/telemetryos/starforge/actions"
)

// phasePermissions is a no-op during overlay builds. Ownership and permission
// operations are applied during packaging by applyImageOwnership, which runs
// against the fully-mounted rootfs where all partition mount points exist and
// usernames resolve from the target's /etc/passwd.
func (b *Builder) phasePermissions(ctx *actions.BuildContext, rootfs string) error {
	if len(ctx.FileOwnerships) > 0 || len(ctx.FilePermissions) > 0 {
		fmt.Printf("    %s\n", dimStyle.Render("deferred to packaging"))
	}
	return nil
}
