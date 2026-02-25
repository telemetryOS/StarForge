package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseSysconfig(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Hostname != "" {
		fmt.Printf("    hostname: %s\n", ctx.Hostname)
		if err := writeFile(filepath.Join(rootfs, "etc/hostname"), ctx.Hostname+"\n"); err != nil {
			return fmt.Errorf("writing hostname: %w", err)
		}
	}

	if ctx.Locale != "" || len(ctx.Locales) > 0 {
		if ctx.Locale != "" {
			fmt.Printf("    locale:   %s\n", ctx.Locale)
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

		localeGen := filepath.Join(rootfs, "etc/locale.gen")
		for _, loc := range allLocales {
			fmt.Printf("    locale-gen: %s\n", loc)
			if err := appendFile(localeGen, fmt.Sprintf("%s UTF-8\n", loc)); err != nil {
				return fmt.Errorf("writing locale.gen: %w", err)
			}
		}
		if err := chrootRun(rootfs, "locale-gen"); err != nil {
			return fmt.Errorf("locale-gen: %w", err)
		}
	}

	if ctx.Timezone != "" {
		fmt.Printf("    timezone: %s\n", ctx.Timezone)
		tzLink := filepath.Join(rootfs, "etc/localtime")
		_ = os.Remove(tzLink)
		if err := os.Symlink(filepath.Join("/usr/share/zoneinfo", ctx.Timezone), tzLink); err != nil {
			return fmt.Errorf("setting timezone: %w", err)
		}
	}

	if ctx.Keymap != "" {
		fmt.Printf("    keymap:   %s\n", ctx.Keymap)
	}

	return nil
}
