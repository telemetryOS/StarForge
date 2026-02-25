package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// installSection holds parsed [Install] directives from a systemd unit file.
type installSection struct {
	WantedBy   []string
	RequiredBy []string
	Alias      []string
	Also       []string
}

// parseInstallSection reads a systemd unit file and extracts [Install] directives.
func parseInstallSection(path string) (installSection, error) {
	f, err := os.Open(path)
	if err != nil {
		return installSection{}, err
	}
	defer f.Close()

	var result installSection
	inInstall := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inInstall = line == "[Install]"
			continue
		}
		if !inInstall {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		// Split on whitespace — systemd allows space-separated lists
		values := strings.Fields(strings.TrimSpace(value))
		switch key {
		case "WantedBy":
			result.WantedBy = append(result.WantedBy, values...)
		case "RequiredBy":
			result.RequiredBy = append(result.RequiredBy, values...)
		case "Alias":
			result.Alias = append(result.Alias, values...)
		case "Also":
			result.Also = append(result.Also, values...)
		}
	}
	return result, scanner.Err()
}

// enableUserUnit creates symlinks for a user-level systemd unit, replicating
// what `systemctl --user enable` does: parse the unit's [Install] section
// and create .wants/.requires symlinks + aliases.
func enableUserUnit(rootfs, user, service string) error {
	userDir := filepath.Join(rootfs, "home", user, ".config/systemd/user")
	unitPath := findUserUnit(rootfs, user, service)
	if unitPath == "" {
		return fmt.Errorf("unit file not found: %s", service)
	}

	install, err := parseInstallSection(unitPath)
	if err != nil {
		return fmt.Errorf("parsing [Install] for %s: %w", service, err)
	}

	// Determine symlink target (what the symlink points to)
	linkTarget := unitPath
	// If in user config dir, use a path relative to the user's systemd dir
	if strings.HasPrefix(unitPath, userDir) {
		// Point to the unit in the same directory
		linkTarget = filepath.Join("/home", user, ".config/systemd/user", service)
	} else {
		// System-provided unit — use absolute path
		rel, _ := filepath.Rel(rootfs, unitPath)
		linkTarget = "/" + rel
	}

	// Create WantedBy symlinks
	for _, target := range install.WantedBy {
		wantsDir := filepath.Join(userDir, target+".wants")
		if err := os.MkdirAll(wantsDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", wantsDir, err)
		}
		link := filepath.Join(wantsDir, service)
		_ = os.Remove(link) // remove existing
		if err := os.Symlink(linkTarget, link); err != nil {
			return fmt.Errorf("creating symlink %s: %w", link, err)
		}
	}

	// Create RequiredBy symlinks
	for _, target := range install.RequiredBy {
		requiresDir := filepath.Join(userDir, target+".requires")
		if err := os.MkdirAll(requiresDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", requiresDir, err)
		}
		link := filepath.Join(requiresDir, service)
		_ = os.Remove(link)
		if err := os.Symlink(linkTarget, link); err != nil {
			return fmt.Errorf("creating symlink %s: %w", link, err)
		}
	}

	// Create Alias symlinks
	for _, alias := range install.Alias {
		link := filepath.Join(userDir, alias)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return err
		}
		_ = os.Remove(link)
		if err := os.Symlink(linkTarget, link); err != nil {
			return fmt.Errorf("creating alias %s: %w", link, err)
		}
	}

	// Create Also enables (recursive)
	for _, also := range install.Also {
		if err := enableUserUnit(rootfs, user, also); err != nil {
			return fmt.Errorf("enabling Also=%s: %w", also, err)
		}
	}

	return nil
}

// disableUserUnit removes symlinks for a user-level systemd unit.
func disableUserUnit(rootfs, user, service string) error {
	userDir := filepath.Join(rootfs, "home", user, ".config/systemd/user")
	unitPath := findUserUnit(rootfs, user, service)
	if unitPath == "" {
		return fmt.Errorf("unit file not found: %s", service)
	}

	install, err := parseInstallSection(unitPath)
	if err != nil {
		return fmt.Errorf("parsing [Install] for %s: %w", service, err)
	}

	for _, target := range install.WantedBy {
		_ = os.Remove(filepath.Join(userDir, target+".wants", service))
	}
	for _, target := range install.RequiredBy {
		_ = os.Remove(filepath.Join(userDir, target+".requires", service))
	}
	for _, alias := range install.Alias {
		_ = os.Remove(filepath.Join(userDir, alias))
	}

	return nil
}

// findUserUnit locates a user unit file, checking user config dir first,
// then system user unit directories.
func findUserUnit(rootfs, user, service string) string {
	candidates := []string{
		filepath.Join(rootfs, "home", user, ".config/systemd/user", service),
		filepath.Join(rootfs, "usr/lib/systemd/user", service),
		filepath.Join(rootfs, "etc/systemd/user", service),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
