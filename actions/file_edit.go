package actions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/telemetryos/starforge/config"
)

type FileEdit struct{}

func (a *FileEdit) Name() string { return "file-edit" }

func (a *FileEdit) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.FileEdit
	if s.Path == "" {
		return fmt.Errorf("file-edit: path is required")
	}

	op := FileEditOp{
		Path:  s.Path,
		Layer: ctx.CurrentLayer,
		Label: step.Label,
	}

	// TaggedContent takes precedence over legacy fields
	if s.Content.Tag != "" {
		switch s.Content.Tag {
		case "append":
			op.Insert = "append"
			op.Content = s.Content.Value
		case "prepend":
			op.Insert = "prepend"
			op.Content = s.Content.Value
		case "before":
			op.Insert = "before"
			op.Pattern = s.Content.Pattern
			op.Match = s.Content.Match
			op.Content = s.Content.Value
		case "after":
			op.Insert = "after"
			op.Pattern = s.Content.Pattern
			op.Match = s.Content.Match
			op.Content = s.Content.Value
		case "truncate_before", "truncate_after":
			op.Truncate = s.Content.Tag
			op.Pattern = s.Content.Pattern
			op.Match = s.Content.Match
		default:
			return fmt.Errorf("file-edit: unknown content tag %q", s.Content.Tag)
		}
	} else {
		// Legacy fields
		op.Content = s.Content.Value
		op.Insert = s.Insert
		op.Truncate = s.Truncate
		op.Pattern = s.Pattern
		op.Match = s.Match
	}

	ctx.FileEdits = append(ctx.FileEdits, op)
	return nil
}

// InsertPattern inserts content before or after lines matching a regex pattern.
// mode is "before" or "after". match limits replacements (0 = all).
func InsertPattern(content, pattern, insert, mode string, match int) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex %q: %w", pattern, err)
	}

	lines := strings.Split(content, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		if re.MatchString(line) && (match == 0 || count < match) {
			count++
			if mode == "before" {
				result = append(result, insert)
				result = append(result, line)
			} else {
				result = append(result, line)
				result = append(result, insert)
			}
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n"), nil
}

// TruncatePattern removes content before or after lines matching a regex pattern.
// mode is "truncate_before" or "truncate_after". match selects which occurrence
// (default 1 if 0).
func TruncatePattern(content, pattern, mode string, match int) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex %q: %w", pattern, err)
	}

	if match == 0 {
		match = 1
	}

	lines := strings.Split(content, "\n")
	count := 0

	for i, line := range lines {
		if re.MatchString(line) {
			count++
			if count == match {
				if mode == "truncate_before" {
					return strings.Join(lines[i:], "\n"), nil
				}
				// truncate_after: keep up to and including the matched line
				return strings.Join(lines[:i+1], "\n"), nil
			}
		}
	}

	return content, nil
}
