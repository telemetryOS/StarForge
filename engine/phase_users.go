package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseUsers(ctx *actions.BuildContext, rootfs string) error {
	// Create explicit groups first (system-group action)
	for _, group := range ctx.Groups {
		args := []string{"groupadd", "-f"}
		if group.System {
			args = append(args, "-r")
		}
		if group.GID != 0 {
			args = append(args, "-g", fmt.Sprintf("%d", group.GID))
		}
		args = append(args, group.Name)
		fmt.Printf("    group: %s\n", group.Name)
		if err := ChrootRun(rootfs, args...); err != nil {
			return fmt.Errorf("creating group %s: %w", group.Name, err)
		}
	}

	for _, user := range ctx.Users {
		groups := ""
		if len(user.Groups) > 0 {
			groups = fmt.Sprintf(" (%s)", strings.Join(user.Groups, ", "))
		}
		label := user.Name
		if user.System {
			label += " (system)"
		}
		fmt.Printf("    %s%s\n", label, groups)

		// Create implicit groups from user group lists
		for _, group := range user.Groups {
			if err := ChrootRun(rootfs, "groupadd", "-f", group); err != nil {
				return fmt.Errorf("creating group %s for user %s: %w", group, user.Name, err)
			}
		}

		args := []string{"useradd"}
		if user.System {
			args = append(args, "-r", "-M") // system user, no home directory
		} else {
			args = append(args, "-m") // create home directory
		}
		if user.Shell != "" {
			args = append(args, "-s", user.Shell)
		}
		if user.UID != 0 {
			args = append(args, "-u", fmt.Sprintf("%d", user.UID))
		}
		if len(user.Groups) > 0 {
			args = append(args, "-G", strings.Join(user.Groups, ","))
		}
		args = append(args, user.Name)

		if err := ChrootRun(rootfs, args...); err != nil {
			return fmt.Errorf("creating user %s: %w", user.Name, err)
		}

		if user.NoPassword {
			if err := ChrootRun(rootfs, "passwd", "-d", user.Name); err != nil {
				return fmt.Errorf("removing password for %s: %w", user.Name, err)
			}
		} else if user.Password != "" {
			cmd := exec.Command(resolveBin("arch-chroot"), rootfs, "chpasswd")
			cmd.Env = vendorEnv()
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", user.Name, user.Password))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("setting password for %s: %w", user.Name, err)
			}
		}
	}
	return nil
}
