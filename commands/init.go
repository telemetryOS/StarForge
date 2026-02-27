package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new StarForge project",
	Long: `Create a new StarForge project with a project configuration file,
initial base layer, and .gitignore.

If a name is provided as an argument, it is used as the project name.
Otherwise, you will be prompted interactively.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Project name
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		fmt.Print("Project name: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading project name: %w", err)
		}
		name = strings.TrimSpace(line)
	}
	if name == "" {
		return fmt.Errorf("project name is required")
	}

	// Description
	fmt.Print("Description (optional): ")
	descLine, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading description: %w", err)
	}
	description := strings.TrimSpace(descLine)

	// Target name
	fmt.Print("First target name [distribution]: ")
	targetLine, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading target name: %w", err)
	}
	targetName := strings.TrimSpace(targetLine)
	if targetName == "" {
		targetName = "distribution"
	}

	// Create project directory
	projectDir := filepath.Join(".", name)
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}

	layerDir := filepath.Join(projectDir, "layers", "base")
	filesDir := filepath.Join(layerDir, "files", "etc")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return fmt.Errorf("creating project structure: %w", err)
	}

	// Generate starforge.yaml
	descField := ""
	if description != "" {
		descField = fmt.Sprintf("description: %q\n", description)
	}

	projectYAML := fmt.Sprintf(`name: %q
%stargets:
  %s:
    layers:
      - ./layers/base
`, name, descField, targetName)

	if err := os.WriteFile(filepath.Join(projectDir, "starforge.yaml"), []byte(projectYAML), 0o644); err != nil {
		return fmt.Errorf("writing starforge.yaml: %w", err)
	}

	// Generate base layer.yaml
	layerYAML := fmt.Sprintf(`steps:
  - action: partition-add
    partitions:
      - name: boot
        filesystem: vfat
        size: 512M
        mount_point: /boot
        type: efi
      - name: root
        filesystem: ext4
        size: 8G
        mount_point: /

  - action: pacman-add
    packages:
      - base
      - linux
      - linux-firmware

  - action: system-hostname
    hostname: %s

  - action: system-locale
    locale: en_US.UTF-8
    locales:
      - en_US.UTF-8 UTF-8

  - action: system-timezone
    timezone: UTC
`, name)

	if err := os.WriteFile(filepath.Join(layerDir, "layer.yaml"), []byte(layerYAML), 0o644); err != nil {
		return fmt.Errorf("writing layer.yaml: %w", err)
	}

	// Generate .gitignore
	gitignore := ".starforge/\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	fmt.Printf("\nCreated project %q in ./%s/\n", name, name)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  starforge build %s\n", targetName)

	return nil
}
