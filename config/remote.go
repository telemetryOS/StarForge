package config

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// IsURL returns true if the path starts with an HTTP(S) origin.
func IsURL(path string) bool {
	return strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://")
}

// FetchFile downloads a URL to a local cache directory.
// Returns the local file path. Reuses cache if present.
func FetchFile(url, cacheDir string) (string, error) {
	hash := sha256.Sum256([]byte(url))
	hexHash := fmt.Sprintf("%x", hash)
	cachedPath := filepath.Join(cacheDir, "downloads", hexHash)

	// Return cached file if it exists
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	if err := os.MkdirAll(filepath.Dir(cachedPath), 0o755); err != nil {
		return "", fmt.Errorf("creating download cache dir: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(cachedPath)
	if err != nil {
		return "", fmt.Errorf("creating cache file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(cachedPath)
		return "", fmt.Errorf("writing cache file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(cachedPath)
		return "", fmt.Errorf("closing cache file: %w", err)
	}

	return cachedPath, nil
}

// FetchRemoteLayer downloads a remote layer.yaml and all its referenced files.
// Returns the local cache directory that mirrors the remote layer structure.
func FetchRemoteLayer(baseURL, cacheDir string) (string, error) {
	// Normalize: strip trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	hash := sha256.Sum256([]byte(baseURL))
	hexHash := fmt.Sprintf("%x", hash)
	mirrorDir := filepath.Join(cacheDir, "remote", hexHash)

	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		return "", fmt.Errorf("creating mirror dir: %w", err)
	}

	// 1. Fetch layer.yaml
	layerURL := baseURL + "/" + LayerFile
	layerPath := filepath.Join(mirrorDir, LayerFile)

	resp, err := http.Get(layerURL)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", layerURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %d", layerURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", layerURL, err)
	}

	if err := os.WriteFile(layerPath, data, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", layerPath, err)
	}

	// 2. Parse layer.yaml and pre-fetch !include files from remote
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parsing remote %s: %w", layerURL, err)
	}

	if err := prefetchIncludes(&doc, baseURL, mirrorDir); err != nil {
		return "", fmt.Errorf("prefetching includes from %s: %w", baseURL, err)
	}

	// 3. Resolve includes (files are now local in mirrorDir)
	if err := ResolveIncludes(&doc, mirrorDir, cacheDir); err != nil {
		return "", fmt.Errorf("resolving includes in remote %s: %w", layerURL, err)
	}

	// 4. Decode to find relative layer_path and script references
	var layer Layer
	if err := doc.Decode(&layer); err != nil {
		return "", fmt.Errorf("decoding remote %s: %w", layerURL, err)
	}

	// Re-write the resolved layer.yaml (with includes expanded)
	resolved, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("marshaling resolved layer: %w", err)
	}
	if err := os.WriteFile(layerPath, resolved, 0o644); err != nil {
		return "", fmt.Errorf("writing resolved layer: %w", err)
	}

	// 5. Collect and fetch all relative paths referenced by steps
	paths := CollectLayerPaths(&layer)
	for _, relPath := range paths {
		if IsURL(relPath) {
			continue // absolute URL — will be fetched individually at action time
		}

		cleanPath := filepath.Clean(relPath)
		fileURL := baseURL + "/" + cleanPath
		localPath := filepath.Join(mirrorDir, cleanPath)

		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return "", fmt.Errorf("creating dir for %s: %w", relPath, err)
		}

		fileResp, err := http.Get(fileURL)
		if err != nil {
			return "", fmt.Errorf("fetching %s: %w", fileURL, err)
		}

		if fileResp.StatusCode != http.StatusOK {
			fileResp.Body.Close()
			return "", fmt.Errorf("fetching %s: HTTP %d", fileURL, fileResp.StatusCode)
		}

		fileData, err := io.ReadAll(fileResp.Body)
		fileResp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", fileURL, err)
		}

		if err := os.WriteFile(localPath, fileData, 0o644); err != nil {
			return "", fmt.Errorf("writing %s: %w", localPath, err)
		}
	}

	return mirrorDir, nil
}

// CollectLayerPaths returns all relative file paths referenced by a layer's steps
// (layer_path and script fields).
func CollectLayerPaths(layer *Layer) []string {
	var paths []string
	for _, step := range layer.Steps {
		if s := step.FileCreate; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.FileEdit; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.Run; s != nil && s.ScriptPath != "" {
			paths = append(paths, s.ScriptPath)
		}
		if s := step.SystemdService; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.SystemdMount; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.SystemdTimer; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.SystemdSocket; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.SystemdSlice; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
		if s := step.SystemdTarget; s != nil && s.LayerPath != "" {
			paths = append(paths, s.LayerPath)
		}
	}
	return paths
}

// prefetchIncludes walks a yaml.Node tree looking for !include tags
// and downloads the referenced files from the remote server into mirrorDir.
// This must be called before ResolveIncludes so the files are available locally.
func prefetchIncludes(node *yaml.Node, baseURL, mirrorDir string) error {
	return prefetchIncludesRecursive(node, baseURL, mirrorDir, 0)
}

func prefetchIncludesRecursive(node *yaml.Node, baseURL, mirrorDir string, depth int) error {
	if depth > maxIncludeDepth {
		return nil
	}

	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			if err := prefetchIncludesRecursive(child, baseURL, mirrorDir, depth); err != nil {
				return err
			}
		}
		return nil
	}

	if node.Kind == yaml.SequenceNode {
		for _, item := range node.Content {
			if item.Tag == "!include" {
				if err := prefetchIncludeNode(item, baseURL, mirrorDir, depth); err != nil {
					return err
				}
			} else {
				if err := prefetchIncludesRecursive(item, baseURL, mirrorDir, depth); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if node.Kind == yaml.MappingNode {
		for i := 1; i < len(node.Content); i += 2 {
			valNode := node.Content[i]
			if valNode.Tag == "!include" {
				if err := prefetchIncludeNode(valNode, baseURL, mirrorDir, depth); err != nil {
					return err
				}
			} else {
				if err := prefetchIncludesRecursive(valNode, baseURL, mirrorDir, depth); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func prefetchIncludeNode(node *yaml.Node, baseURL, mirrorDir string, depth int) error {
	var filePath string

	switch node.Kind {
	case yaml.ScalarNode:
		filePath = node.Value
	case yaml.MappingNode:
		// Peek at layer_path without clearing the tag
		for i := 0; i < len(node.Content)-1; i += 2 {
			if node.Content[i].Value == "layer_path" {
				filePath = node.Content[i+1].Value
				break
			}
		}
	}

	if filePath == "" || IsURL(filePath) {
		return nil // URL includes are fetched directly, skip
	}

	cleanPath := filepath.Clean(filePath)
	fileURL := baseURL + "/" + cleanPath
	localPath := filepath.Join(mirrorDir, cleanPath)

	// Skip if already downloaded
	if _, err := os.Stat(localPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating dir for include %s: %w", cleanPath, err)
	}

	resp, err := http.Get(fileURL)
	if err != nil {
		return fmt.Errorf("fetching include %s: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching include %s: HTTP %d", fileURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading include %s: %w", fileURL, err)
	}

	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return fmt.Errorf("writing include %s: %w", localPath, err)
	}

	// Recursively prefetch includes in the downloaded file
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil // not valid YAML, let ResolveIncludes handle the error
	}

	return prefetchIncludesRecursive(&doc, baseURL, mirrorDir, depth+1)
}
