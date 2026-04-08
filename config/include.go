package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxIncludeDepth = 10

// ResolveIncludes walks a yaml.Node tree, finds !include tags,
// and replaces them with the included content. Handles list splicing.
func ResolveIncludes(root *yaml.Node, layerDir string, cacheDir string) error {
	return resolveIncludesRecursive(root, layerDir, cacheDir, 0)
}

func resolveIncludesRecursive(node *yaml.Node, layerDir string, cacheDir string, depth int) error {
	if depth > maxIncludeDepth {
		return fmt.Errorf("!include depth limit exceeded (%d levels)", maxIncludeDepth)
	}

	// Document nodes wrap the actual content
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			if err := resolveIncludesRecursive(child, layerDir, cacheDir, depth); err != nil {
				return err
			}
		}
		return nil
	}

	// Sequence nodes: check each element for !include, splice results
	if node.Kind == yaml.SequenceNode {
		var newContent []*yaml.Node
		for _, item := range node.Content {
			if item.Tag == "!include" {
				resolved, err := resolveIncludeNode(item, layerDir, cacheDir, depth)
				if err != nil {
					return err
				}
				// Splice: if the resolved content is a sequence, inline its items
				if resolved.Kind == yaml.SequenceNode {
					newContent = append(newContent, resolved.Content...)
				} else {
					newContent = append(newContent, resolved)
				}
			} else {
				if err := resolveIncludesRecursive(item, layerDir, cacheDir, depth); err != nil {
					return err
				}
				newContent = append(newContent, item)
			}
		}
		node.Content = newContent
		return nil
	}

	// Mapping nodes: check values for !include
	if node.Kind == yaml.MappingNode {
		for i := 1; i < len(node.Content); i += 2 {
			valNode := node.Content[i]
			if valNode.Tag == "!include" {
				resolved, err := resolveIncludeNode(valNode, layerDir, cacheDir, depth)
				if err != nil {
					return err
				}
				node.Content[i] = resolved
			} else {
				if err := resolveIncludesRecursive(valNode, layerDir, cacheDir, depth); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return nil
}

// resolveIncludeNode processes a single !include node and returns the replacement.
func resolveIncludeNode(node *yaml.Node, layerDir string, cacheDir string, depth int) (*yaml.Node, error) {
	var filePath, yamlPath string

	switch node.Kind {
	case yaml.ScalarNode:
		// !include ./file.yaml
		filePath = node.Value

	case yaml.MappingNode:
		// !include
		//   layer_path: ./file.yaml
		//   yaml_path: steps
		var inc struct {
			LayerPath string `yaml:"layer_path"`
			YAMLPath  string `yaml:"yaml_path"`
		}
		node.Tag = "" // clear tag so Decode works
		if err := node.Decode(&inc); err != nil {
			return nil, fmt.Errorf("!include: invalid mapping: %w", err)
		}
		filePath = inc.LayerPath
		yamlPath = inc.YAMLPath

	default:
		return nil, fmt.Errorf("!include: expected scalar or mapping, got kind %d", node.Kind)
	}

	if filePath == "" {
		return nil, fmt.Errorf("!include: file path is required")
	}

	// Resolve the file path
	var resolvedPath string
	if IsURL(filePath) {
		if cacheDir == "" {
			return nil, fmt.Errorf("!include: URL includes require a cache directory")
		}
		var err error
		resolvedPath, err = FetchFile(filePath, cacheDir)
		if err != nil {
			return nil, fmt.Errorf("!include: fetching %s: %w", filePath, err)
		}
	} else {
		cleanPath := filepath.Clean(filePath)
		if isPathTraversal(cleanPath) {
			return nil, fmt.Errorf("!include: path %q escapes the layer directory", filePath)
		}
		resolvedPath = filepath.Join(layerDir, cleanPath)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("!include: reading %s: %w", filePath, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("!include: parsing %s: %w", filePath, err)
	}

	// Get the actual content node (unwrap document)
	content := &doc
	if content.Kind == yaml.DocumentNode && len(content.Content) > 0 {
		content = content.Content[0]
	}

	// Navigate to yaml_path if specified
	if yamlPath != "" {
		content, err = navigateYAMLPath(content, yamlPath)
		if err != nil {
			return nil, fmt.Errorf("!include %s: %w", filePath, err)
		}
	}

	// Recursive includes resolve relative to the included file's directory
	includeLayerDir := filepath.Dir(resolvedPath)

	// Recurse into the included content
	if err := resolveIncludesRecursive(content, includeLayerDir, cacheDir, depth+1); err != nil {
		return nil, fmt.Errorf("!include %s: %w", filePath, err)
	}

	return content, nil
}

// navigateYAMLPath navigates a yaml.Node tree using a dot-separated path.
// Supports mapping keys and sequence indices.
func navigateYAMLPath(node *yaml.Node, path string) (*yaml.Node, error) {
	parts := strings.Split(path, ".")
	current := node

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch current.Kind {
		case yaml.MappingNode:
			found := false
			for i := 0; i < len(current.Content)-1; i += 2 {
				if current.Content[i].Value == part {
					current = current.Content[i+1]
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("yaml_path: key %q not found", part)
			}

		case yaml.SequenceNode:
			idx, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("yaml_path: expected integer index for sequence, got %q", part)
			}
			if idx < 0 || idx >= len(current.Content) {
				return nil, fmt.Errorf("yaml_path: index %d out of range (length %d)", idx, len(current.Content))
			}
			current = current.Content[idx]

		default:
			return nil, fmt.Errorf("yaml_path: cannot navigate into scalar at %q", part)
		}
	}

	return current, nil
}
