package actions

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateInstallPath(action, field, value string) error {
	if value == "" {
		return fmt.Errorf("%s: %s is required", action, field)
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("%s: %s must be absolute: %q", action, field, value)
	}
	clean := filepath.Clean(value)
	if clean != value {
		return fmt.Errorf("%s: %s must be clean: %q", action, field, value)
	}
	for _, elem := range strings.Split(value, string(filepath.Separator)) {
		if elem == ".." {
			return fmt.Errorf("%s: %s must not contain '..': %q", action, field, value)
		}
	}
	if strings.ContainsAny(value, "\n\r\t %") {
		return fmt.Errorf("%s: %s must not contain whitespace or systemd specifiers: %q", action, field, value)
	}
	return nil
}
