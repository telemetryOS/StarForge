package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseScripts(ctx *actions.BuildContext, rootfs string) error {
	for _, script := range ctx.Scripts {
		// Determine display label
		label := script.Label
		if label == "" {
			label = script.Script
		}
		if label == "" {
			label = "(inline)"
		}
		if script.User != "" {
			out.Styled(
				fmt.Sprintf("    %s  %s", label, dimStyle.Render("user: "+script.User)),
				fmt.Sprintf("    %s  user: %s", label, script.User),
			)
		} else {
			out.Info("%s", label)
		}

		// Use /var/tmp instead of /tmp — arch-chroot mounts a tmpfs on /tmp
		// which would hide any files we write to the overlay's /tmp.
		if err := os.MkdirAll(filepath.Join(rootfs, "var", "tmp"), 0o755); err != nil {
			return fmt.Errorf("creating var/tmp dir: %w", err)
		}

		// Build script prelude with sf_set/sf_get + variable declarations
		prelude := buildScriptPrelude(ctx.Vars)

		var tmpScript, chrootScriptPath string

		if script.Script != "" {
			// File-based: read from layer dir, prepend prelude, write to chroot
			scriptPath := filepath.Join(script.LayerDir, script.Script)
			scriptData, err := os.ReadFile(scriptPath)
			if err != nil {
				return fmt.Errorf("reading script %s: %w", script.Script, err)
			}
			content := injectPrelude(string(scriptData), prelude)
			tmpScript = filepath.Join(rootfs, "var/tmp", filepath.Base(script.Script))
			if err := os.WriteFile(tmpScript, []byte(content), 0o755); err != nil {
				return fmt.Errorf("writing script %s: %w", script.Script, err)
			}
			chrootScriptPath = filepath.Join("/var/tmp", filepath.Base(script.Script))
		} else {
			// Inline: write prelude + content to temp file
			content := injectPrelude(script.Content, prelude)
			tmpScript = filepath.Join(rootfs, "var/tmp", "starforge-inline.sh")
			if err := os.WriteFile(tmpScript, []byte(content), 0o755); err != nil {
				return fmt.Errorf("writing inline script: %w", err)
			}
			chrootScriptPath = "/var/tmp/starforge-inline.sh"
		}

		if err := run("chmod", "+x", tmpScript); err != nil {
			return fmt.Errorf("making script executable: %w", err)
		}

		// Merge env: target-level + step-level (step overrides target)
		env := mergeScriptEnv(ctx.Env, script.Env)

		var runErr error
		if script.User != "" {
			// Use "su user" (without "-") so the environment set by ctx.Env
			// and script.Env reaches the script. "su -" starts a login shell
			// that strips most environment variables.
			runErr = chrootRunWithEnv(rootfs, env, "su", script.User, "-s", "/bin/bash", "-c", chrootScriptPath)
		} else {
			runErr = chrootRunWithEnv(rootfs, env, chrootScriptPath)
		}
		os.Remove(tmpScript)
		if runErr != nil {
			return fmt.Errorf("running script %s: %w", label, runErr)
		}
	}
	return nil
}
