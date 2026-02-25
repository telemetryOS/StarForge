package engine

import (
	"fmt"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseServices(ctx *actions.BuildContext, rootfs string) error {
	for _, svc := range ctx.Services.Mask {
		fmt.Printf("    mask:    %s\n", svc)
		if err := chrootRun(rootfs, "systemctl", "mask", svc); err != nil {
			return fmt.Errorf("masking %s: %w", svc, err)
		}
	}

	for _, svc := range ctx.Services.Enable {
		fmt.Printf("    enable:  %s\n", svc)
		if err := chrootRun(rootfs, "systemctl", "enable", svc); err != nil {
			return fmt.Errorf("enabling %s: %w", svc, err)
		}
	}

	for _, svc := range ctx.Services.Disable {
		fmt.Printf("    disable: %s\n", svc)
		if err := chrootRun(rootfs, "systemctl", "disable", svc); err != nil {
			return fmt.Errorf("disabling %s: %w", svc, err)
		}
	}

	// User-level enable: parse [Install] sections and create symlinks
	for _, op := range ctx.Services.UserEnable {
		fmt.Printf("    enable:  %s (user: %s)\n", op.Service, op.User)
		if err := enableUserUnit(rootfs, op.User, op.Service); err != nil {
			return fmt.Errorf("enabling user unit %s for %s: %w", op.Service, op.User, err)
		}
	}

	// User-level disable: remove symlinks
	for _, op := range ctx.Services.UserDisable {
		fmt.Printf("    disable: %s (user: %s)\n", op.Service, op.User)
		if err := disableUserUnit(rootfs, op.User, op.Service); err != nil {
			return fmt.Errorf("disabling user unit %s for %s: %w", op.Service, op.User, err)
		}
	}

	if ctx.DefaultTarget != "" {
		fmt.Printf("    default: %s\n", ctx.DefaultTarget)
		if err := chrootRun(rootfs, "systemctl", "set-default", ctx.DefaultTarget); err != nil {
			return fmt.Errorf("setting default target %s: %w", ctx.DefaultTarget, err)
		}
	}

	return nil
}
