package upgrade

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func Upgrade() error {
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current binary path: %w", err)
	}

	if strings.Contains(currentBinary, "/go-build") {
		fmt.Println("Cannot upgrade when running via 'go run'")
		return nil
	}

	fmt.Println("Fetching latest version...")

	tag, err := getLatestTag()
	if err != nil {
		return fmt.Errorf("failed to get latest tag: %w", err)
	}

	fmt.Printf("Latest version: %s\n", tag)

	tmpDir, err := os.MkdirTemp("", "starforge-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Println("Downloading source...")

	repoURL := "https://github.com/telemetryOS/StarForge.git"
	if err := cloneTag(repoURL, tag, tmpDir); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	fmt.Println("Building new version...")

	binaryPath, err := buildBinary(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	fmt.Println("Replacing binary...")

	os.Remove(currentBinary)

	src, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(currentBinary)
	if err != nil {
		return fmt.Errorf("failed to create new binary: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	if err := dst.Chmod(0755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	fmt.Printf("Upgrade complete to %s\n", tag)

	return nil
}

func getLatestTag() (string, error) {
	cmd := exec.Command("git", "ls-remote", "--tags", "--refs", "https://github.com/telemetryOS/StarForge.git")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	versionRegex := regexp.MustCompile(`refs/tags/(v\d+\.\d+\.\d+)$`)
	lines := strings.Split(string(output), "\n")

	var latestTag string
	for _, line := range lines {
		matches := versionRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			latestTag = matches[1]
		}
	}

	if latestTag == "" {
		return "", fmt.Errorf("no version tags found")
	}

	return latestTag, nil
}

func cloneTag(repoURL, tag, destPath string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", tag, repoURL, destPath)
	return cmd.Run()
}

func buildBinary(repoPath string) (string, error) {
	outputPath := filepath.Join(repoPath, "starforge")
	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/starforge")
	cmd.Dir = repoPath

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return outputPath, nil
}
