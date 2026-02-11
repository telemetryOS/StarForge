package actions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/telemetryos/starforge/config"
)

// RenderUnit renders a systemd unit file from a map of section names to key-value pairs.
func RenderUnit(sections map[string]map[string]any) string {
	// Sort section names for deterministic output
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for i, name := range names {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("[%s]\n", name))

		sec := sections[name]
		keys := make([]string, 0, len(sec))
		for k := range sec {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := sec[k]
			switch val := v.(type) {
			case config.ReplaceValue:
				// Clear-then-set: emit Key= to clear the parent value,
				// then Key=value to set the new one. This is the standard
				// systemd drop-in pattern for overriding inherited directives.
				b.WriteString(fmt.Sprintf("%s=\n", k))
				b.WriteString(fmt.Sprintf("%s=%v\n", k, val.Value))
			case []any:
				for _, item := range val {
					b.WriteString(fmt.Sprintf("%s=%v\n", k, item))
				}
			case []string:
				for _, item := range val {
					b.WriteString(fmt.Sprintf("%s=%s\n", k, item))
				}
			default:
				b.WriteString(fmt.Sprintf("%s=%v\n", k, val))
			}
		}
	}

	return b.String()
}
