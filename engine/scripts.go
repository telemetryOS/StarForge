package engine

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/telemetryos/starforge/config"
)

// buildScriptPrelude generates a bash prelude that defines sf_set, sf_get,
// and declares all collected variables for use inside chroot scripts.
func buildScriptPrelude(vars map[string]string) string {
	if len(vars) == 0 {
		return "# starforge prelude\nsf_set() { :; }\nsf_get() { printf '%s' \"${__sf_vars[$1]}\"; }\ndeclare -A __sf_vars=()\n"
	}
	var b strings.Builder
	b.WriteString("# starforge prelude\n")
	b.WriteString("sf_set() { :; }\n")
	b.WriteString("sf_get() { printf '%s' \"${__sf_vars[$1]}\"; }\n")
	b.WriteString("declare -A __sf_vars=(")
	// Sort keys for deterministic output
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		// Escape single quotes in values: replace ' with '\''
		v := strings.ReplaceAll(vars[k], "'", "'\\''")
		fmt.Fprintf(&b, " [%s]='%s'", k, v)
	}
	b.WriteString(" )\n")
	return b.String()
}

// injectPrelude prepends the prelude after the shebang line (if present).
func injectPrelude(script, prelude string) string {
	if strings.HasPrefix(script, "#!") {
		if idx := strings.Index(script, "\n"); idx != -1 {
			return script[:idx+1] + prelude + script[idx+1:]
		}
	}
	return prelude + script
}

// mergeScriptEnv merges target-level and step-level env maps.
// Step-level values override target-level values.
func mergeScriptEnv(targetEnv, stepEnv map[string]string) map[string]string {
	if len(targetEnv) == 0 && len(stepEnv) == 0 {
		return nil
	}
	merged := make(map[string]string, len(targetEnv)+len(stepEnv))
	for k, v := range targetEnv {
		merged[k] = v
	}
	for k, v := range stepEnv {
		merged[k] = v
	}
	return merged
}

// chrootRunWithEnv executes a command inside the rootfs with extra env vars.
func chrootRunWithEnv(rootfs string, env map[string]string, args ...string) error {
	cmdArgs := append([]string{rootfs}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if out != nil {
		w := out.ProcessWriter()
		cmd.Stdout = w
		cmd.Stderr = w
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// executeLayerRun runs a layer-run script on the host during Collect.
// It captures sf_set calls from the script output and updates vars.
func (b *Builder) executeLayerRun(step config.Step, layerDir string, vars, targetEnv map[string]string) error {
	s := step.LayerRun
	if s.Script == "" && s.ScriptPath == "" {
		return fmt.Errorf("script or script_path is required")
	}
	if s.Script != "" && s.ScriptPath != "" {
		return fmt.Errorf("script and script_path are mutually exclusive")
	}

	// Create temp file for sf_set output
	outputFile, err := os.CreateTemp("", "starforge-layerrun-output-*")
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	outputPath := outputFile.Name()
	outputFile.Close()
	defer os.Remove(outputPath)

	// Build the prelude with sf_set, sf_get, and current vars
	var prelude strings.Builder
	prelude.WriteString("# starforge layer-run prelude\n")
	fmt.Fprintf(&prelude, "__sf_output=%q\n", outputPath)
	prelude.WriteString("sf_set() { echo \"$1=$2\" >> \"$__sf_output\"; }\n")
	prelude.WriteString("sf_get() { printf '%s' \"${__sf_vars[$1]}\"; }\n")
	prelude.WriteString("declare -A __sf_vars=(")
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.ReplaceAll(vars[k], "'", "'\\''")
		fmt.Fprintf(&prelude, " [%s]='%s'", k, v)
	}
	prelude.WriteString(" )\n")

	// Get the script content
	var scriptContent string
	if s.ScriptPath != "" {
		data, err := os.ReadFile(filepath.Join(layerDir, s.ScriptPath))
		if err != nil {
			return fmt.Errorf("reading script %s: %w", s.ScriptPath, err)
		}
		scriptContent = string(data)
	} else {
		scriptContent = s.Script
	}

	// Inject prelude into script
	fullScript := injectPrelude(scriptContent, prelude.String())

	// Write to temp file
	tmpScript, err := os.CreateTemp("", "starforge-layerrun-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp script: %w", err)
	}
	tmpScriptPath := tmpScript.Name()
	defer os.Remove(tmpScriptPath)

	if _, err := tmpScript.WriteString(fullScript); err != nil {
		tmpScript.Close()
		return fmt.Errorf("writing temp script: %w", err)
	}
	tmpScript.Close()

	if err := os.Chmod(tmpScriptPath, 0o755); err != nil {
		return fmt.Errorf("making script executable: %w", err)
	}

	// Build env: host env + target env + step env + STARFORGE_VAR_* vars
	cmd := exec.Command("bash", tmpScriptPath)
	cmd.Dir = layerDir
	cmd.Env = os.Environ()
	for k, v := range targetEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	for k, v := range s.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	for k, v := range vars {
		cmd.Env = append(cmd.Env, "STARFORGE_VAR_"+strings.ToUpper(k)+"="+v)
	}
	// Route output through the TUI output system when available.
	if out != nil {
		w := out.ProcessWriter()
		cmd.Stdout = w
		cmd.Stderr = w
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("script failed: %w", err)
	}

	// Parse output file: KEY=VALUE lines → update vars
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No output file = no vars set
		}
		return fmt.Errorf("reading output: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(outputData)))
	for scanner.Scan() {
		line := scanner.Text()
		if key, value, ok := strings.Cut(line, "="); ok && key != "" {
			vars[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("parsing output: %w", err)
	}

	return nil
}

// runLayerScript executes a host-side build script in the resolved source directory.
// The script is either inline (step.LayerScript) or read from a file relative to layerDir.
func runLayerScript(step config.Step, layerDir, sourceDir string) error {
	var script string
	if step.LayerScriptPath != "" {
		data, err := os.ReadFile(filepath.Join(layerDir, step.LayerScriptPath))
		if err != nil {
			return fmt.Errorf("reading layer script %s: %w", step.LayerScriptPath, err)
		}
		script = string(data)
	} else {
		script = step.LayerScript
	}

	// Write script to temp file in source dir
	tmpScript := filepath.Join(sourceDir, ".starforge-build.sh")
	if err := os.WriteFile(tmpScript, []byte(script), 0o755); err != nil {
		return fmt.Errorf("writing build script: %w", err)
	}
	defer os.Remove(tmpScript)

	// Execute in source directory on host
	cmd := exec.Command("bash", tmpScript)
	cmd.Dir = sourceDir
	if out != nil {
		w := out.ProcessWriter()
		cmd.Stdout = w
		cmd.Stderr = w
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}
