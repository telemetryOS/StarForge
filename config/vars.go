package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// varPattern matches ${{ var_name }} references in YAML strings.
var varPattern = regexp.MustCompile(`\$\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// VarPattern returns the compiled regex for variable substitution.
func VarPattern() *regexp.Regexp {
	return varPattern
}

// SubstituteVars walks a yaml.Node tree and replaces all ${{ var }} references
// in scalar string values with the corresponding values from vars.
// Returns an error if any referenced variable is not defined.
func SubstituteVars(node *yaml.Node, vars map[string]string) error {
	visited := make(map[*yaml.Node]bool)
	return substituteVarsRecursive(node, vars, visited)
}

func substituteVarsRecursive(node *yaml.Node, vars map[string]string, visited map[*yaml.Node]bool) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := substituteVarsRecursive(child, vars, visited); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content)-1; i += 2 {
			// Substitute in both keys and values
			if err := substituteVarsRecursive(node.Content[i], vars, visited); err != nil {
				return err
			}
			if err := substituteVarsRecursive(node.Content[i+1], vars, visited); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := substituteVarsRecursive(child, vars, visited); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag == "!include" {
			// Don't substitute inside !include tags — they are processed separately
			return nil
		}
		if !strings.Contains(node.Value, "${{") {
			return nil
		}
		var firstErr error
		result := varPattern.ReplaceAllStringFunc(node.Value, func(match string) string {
			sub := varPattern.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			name := sub[1]
			val, ok := vars[name]
			if !ok {
				if firstErr == nil {
					firstErr = fmt.Errorf("undefined variable: %s", name)
				}
				return match
			}
			return val
		})
		if firstErr != nil {
			return firstErr
		}
		node.Value = result
	case yaml.AliasNode:
		// Aliases point to other nodes; substitute in the alias target.
		// Track visited nodes to prevent infinite loops from circular aliases.
		if node.Alias != nil && !visited[node.Alias] {
			visited[node.Alias] = true
			return substituteVarsRecursive(node.Alias, vars, visited)
		}
	}
	return nil
}

// DeepCopyNode creates a deep copy of a yaml.Node tree.
func DeepCopyNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cp := &yaml.Node{
		Kind:        node.Kind,
		Style:       node.Style,
		Tag:         node.Tag,
		Value:       node.Value,
		Anchor:      node.Anchor,
		HeadComment: node.HeadComment,
		LineComment: node.LineComment,
		FootComment: node.FootComment,
		Line:        node.Line,
		Column:      node.Column,
	}
	if len(node.Content) > 0 {
		cp.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			cp.Content[i] = DeepCopyNode(child)
		}
	}
	// Note: Alias references are not deep-copied to avoid infinite recursion.
	// This is fine because we substitute in the alias target directly.
	cp.Alias = node.Alias
	return cp
}

