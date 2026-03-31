package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseSysconfig(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Hostname != "" {
		out.Info("hostname: %s", ctx.Hostname)
		if err := writeFile(filepath.Join(rootfs, "etc/hostname"), ctx.Hostname+"\n"); err != nil {
			return fmt.Errorf("writing hostname: %w", err)
		}
	}

	if ctx.Locale != "" || len(ctx.Locales) > 0 {
		if ctx.Locale != "" {
			out.Info("locale:   %s", ctx.Locale)
			if err := writeFile(filepath.Join(rootfs, "etc/locale.conf"), fmt.Sprintf("LANG=%s\n", ctx.Locale)); err != nil {
				return fmt.Errorf("writing locale.conf: %w", err)
			}
		}

		// Collect all locales: primary (auto-included) + explicit list, deduplicated
		seen := make(map[string]bool)
		var allLocales []string
		if ctx.Locale != "" {
			seen[ctx.Locale] = true
			allLocales = append(allLocales, ctx.Locale)
		}
		for _, loc := range ctx.Locales {
			if !seen[loc] {
				seen[loc] = true
				allLocales = append(allLocales, loc)
			}
		}

		// Build the full locale.gen content at once and overwrite the file
		// so that re-running the phase produces the same result (idempotent).
		localeGen := filepath.Join(rootfs, "etc/locale.gen")
		var localeGenContent strings.Builder
		for _, loc := range allLocales {
			out.Info("locale-gen: %s", loc)
			fmt.Fprintf(&localeGenContent, "%s UTF-8\n", loc)
		}
		if err := writeFile(localeGen, localeGenContent.String()); err != nil {
			return fmt.Errorf("writing locale.gen: %w", err)
		}
		if err := ChrootRun(rootfs, "locale-gen"); err != nil {
			return fmt.Errorf("locale-gen: %w", err)
		}
	}

	if ctx.Timezone != "" {
		out.Info("timezone: %s", ctx.Timezone)
		tzLink := filepath.Join(rootfs, "etc/localtime")
		if err := os.MkdirAll(filepath.Dir(tzLink), 0o755); err != nil {
			return fmt.Errorf("creating etc for timezone: %w", err)
		}
		_ = os.Remove(tzLink)
		if err := os.Symlink(filepath.Join("/usr/share/zoneinfo", ctx.Timezone), tzLink); err != nil {
			return fmt.Errorf("setting timezone: %w", err)
		}
	}

	if ctx.Keymap != "" {
		out.Info("keymap:   %s", ctx.Keymap)
	}

	return nil
}
