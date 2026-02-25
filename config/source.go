package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsGitSource returns true if the source looks like a git repo URL.
// Detects: *.git, *.git#ref
func IsGitSource(source string) bool {
	base := source
	if idx := strings.Index(source, "#"); idx != -1 {
		base = source[:idx]
	}
	return strings.HasSuffix(base, ".git")
}

// ParseGitSource splits "url.git#ref" into (repo, ref).
// If no ref is specified, ref is empty.
func ParseGitSource(source string) (repo, ref string) {
	if idx := strings.Index(source, "#"); idx != -1 {
		return source[:idx], source[idx+1:]
	}
	return source, ""
}

// IsArchiveSource returns true if the source is a downloadable archive URL.
func IsArchiveSource(source string) bool {
	clean := source
	// Strip query string and fragment
	if idx := strings.Index(clean, "?"); idx != -1 {
		clean = clean[:idx]
	}
	if idx := strings.Index(clean, "#"); idx != -1 {
		clean = clean[:idx]
	}
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.bz2", ".tar.xz", ".zip"} {
		if strings.HasSuffix(clean, ext) {
			return true
		}
	}
	return false
}

// ResolveSource fetches a git repo or archive and returns the local directory.
// Cache layout: <cacheDir>/sources/<sha256(source)>/
func ResolveSource(source, cacheDir string) (string, error) {
	switch {
	case IsGitSource(source):
		return resolveGitSource(source, cacheDir)
	case IsArchiveSource(source):
		return resolveArchiveSource(source, cacheDir)
	default:
		return "", fmt.Errorf("unrecognized source type: %s", source)
	}
}

// resolveGitSource clones a git repo (shallow) into the source cache.
func resolveGitSource(source, cacheDir string) (string, error) {
	repo, ref := ParseGitSource(source)

	hash := sha256.Sum256([]byte(source))
	hexHash := fmt.Sprintf("%x", hash)
	sourceDir := filepath.Join(cacheDir, "sources", hexHash)

	// Check for resolved marker
	marker := filepath.Join(sourceDir, ".resolved")
	if _, err := os.Stat(marker); err == nil {
		return sourceDir, nil
	}

	// Clean any partial clone
	os.RemoveAll(sourceDir)
	if err := os.MkdirAll(filepath.Dir(sourceDir), 0o755); err != nil {
		return "", fmt.Errorf("creating source cache dir: %w", err)
	}

	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, repo, sourceDir)

	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(sourceDir)
		return "", fmt.Errorf("cloning %s: %w", source, err)
	}

	// Read the resolved commit hash
	commitCmd := exec.Command("git", "-C", sourceDir, "rev-parse", "HEAD")
	commitOut, err := commitCmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading commit hash: %w", err)
	}
	commit := strings.TrimSpace(string(commitOut))

	// Write resolved marker
	if err := os.WriteFile(marker, []byte(commit+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("writing resolved marker: %w", err)
	}

	return sourceDir, nil
}

// resolveArchiveSource downloads and extracts an archive into the source cache.
func resolveArchiveSource(source, cacheDir string) (string, error) {
	hash := sha256.Sum256([]byte(source))
	hexHash := fmt.Sprintf("%x", hash)
	sourceDir := filepath.Join(cacheDir, "sources", hexHash)

	// Check for resolved marker
	marker := filepath.Join(sourceDir, ".resolved")
	if _, err := os.Stat(marker); err == nil {
		return sourceDir, nil
	}

	// Download the archive
	cachedFile, err := FetchFile(source, cacheDir)
	if err != nil {
		return "", fmt.Errorf("downloading archive %s: %w", source, err)
	}

	// Clean any partial extraction
	os.RemoveAll(sourceDir)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return "", fmt.Errorf("creating source dir: %w", err)
	}

	// Detect archive type and extract
	clean := source
	if idx := strings.Index(clean, "?"); idx != -1 {
		clean = clean[:idx]
	}
	if idx := strings.Index(clean, "#"); idx != -1 {
		clean = clean[:idx]
	}

	var extractCmd *exec.Cmd
	switch {
	case strings.HasSuffix(clean, ".tar.gz"), strings.HasSuffix(clean, ".tgz"):
		extractCmd = exec.Command("tar", "xzf", cachedFile, "-C", sourceDir, "--strip-components=1")
	case strings.HasSuffix(clean, ".tar.bz2"):
		extractCmd = exec.Command("tar", "xjf", cachedFile, "-C", sourceDir, "--strip-components=1")
	case strings.HasSuffix(clean, ".tar.xz"):
		extractCmd = exec.Command("tar", "xJf", cachedFile, "-C", sourceDir, "--strip-components=1")
	case strings.HasSuffix(clean, ".zip"):
		extractCmd = exec.Command("unzip", "-o", "-d", sourceDir, cachedFile)
	default:
		return "", fmt.Errorf("unsupported archive format: %s", source)
	}

	extractCmd.Stdout = os.Stdout
	extractCmd.Stderr = os.Stderr
	if err := extractCmd.Run(); err != nil {
		os.RemoveAll(sourceDir)
		return "", fmt.Errorf("extracting %s: %w", source, err)
	}

	// Write resolved marker
	if err := os.WriteFile(marker, []byte(hexHash+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("writing resolved marker: %w", err)
	}

	return sourceDir, nil
}

// ReadSourceResolved reads the .resolved marker from a source directory.
// Returns the commit hash (for git) or content hash (for archives).
func ReadSourceResolved(sourceDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(sourceDir, ".resolved"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
