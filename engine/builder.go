package engine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	phaseStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	layerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	cachedStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Builder orchestrates the build process for a project.
type Builder struct {
	project *config.Project
	DryRun  bool // skip layer-run and file I/O (for inspect)
}

// NewBuilder creates a new builder for the given project.
func NewBuilder(project *config.Project) *Builder {
	return &Builder{project: project}
}

// Build collects all layer actions into a BuildContext, then executes
// the resolved state in phase order using overlayfs for caching.
// The clean flag forces a full rebuild by deleting the cache first.
func (b *Builder) Build(targetName string, target config.Target, clean bool) error {
	fmt.Println(headerStyle.Render(fmt.Sprintf("Building target: %s", targetName)))
	fmt.Println()

	// Create build directory
	buildDir := b.project.TargetBuildDir(targetName)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return fmt.Errorf("creating build directory: %w", err)
	}

	// Phase 1: Collect — process all layers and build the context
	ctx, err := b.Collect(target, true)
	if err != nil {
		return err
	}

	// Phase 2: Execute — run each phase in order with overlay caching
	overlay := NewOverlayManager(buildDir)

	if clean {
		fmt.Println(headerStyle.Render("Cleaning cache"))
		if err := overlay.CleanCache(); err != nil {
			return fmt.Errorf("cleaning cache: %w", err)
		}
		fmt.Println()
	}

	if err := b.execute(ctx, buildDir, overlay); err != nil {
		overlay.Unmount()
		return err
	}

	fmt.Println(successStyle.Render(fmt.Sprintf("Build complete: %s", buildDir)))
	return nil
}

// Collect processes all layers and returns the resolved BuildContext.
// If verbose is true, prints progress to stdout.
func (b *Builder) Collect(target config.Target, verbose bool) (*actions.BuildContext, error) {
	if verbose {
		fmt.Println(headerStyle.Render("Collecting layers"))
		fmt.Println()
	}

	ctx := actions.NewBuildContext()
	ctx.DryRun = b.DryRun

	// Set up download cache dir for URL support
	cacheDir := filepath.Join(b.project.BuildDir(), "cache")
	ctx.DownloadCacheDir = cacheDir

	// Initialize variable scope from target args
	vars := make(map[string]string)
	for k, v := range target.Args {
		vars[k] = v
	}

	// Substitute variables in target env and store in ctx
	if len(target.Env) > 0 {
		ctx.Env = make(map[string]string, len(target.Env))
		for k, v := range target.Env {
			resolved, err := substituteString(v, vars)
			if err != nil {
				return nil, fmt.Errorf("target env %s: %w", k, err)
			}
			ctx.Env[k] = resolved
		}
	}

	for i, layerPath := range target.Layers {
		resolvedPath := b.project.ResolveLayerPath(layerPath)

		// For remote/git/archive layers, fall back to the non-raw path
		// since variable substitution in remote layers is not supported
		var rawLayer *config.RawLayer
		var legacyLayer *config.Layer
		var layerDir string

		switch {
		case config.IsGitSource(resolvedPath) || config.IsArchiveSource(resolvedPath):
			sourceDir, fetchErr := config.ResolveSource(resolvedPath, cacheDir)
			if fetchErr != nil {
				return nil, fmt.Errorf("fetching source layer %s: %w", layerPath, fetchErr)
			}
			var err error
			rawLayer, err = config.LoadLayerRaw(sourceDir, cacheDir)
			if err != nil {
				return nil, fmt.Errorf("loading layer %s: %w", layerPath, err)
			}
			layerDir = rawLayer.Dir
		case config.IsURL(resolvedPath):
			// Remote layers: use LoadLayer (includes pre-fetching of referenced files)
			mirrorDir, fetchErr := config.FetchRemoteLayer(resolvedPath, cacheDir)
			if fetchErr != nil {
				return nil, fmt.Errorf("fetching remote layer %s: %w", layerPath, fetchErr)
			}
			var err error
			legacyLayer, err = config.LoadLayer(mirrorDir, cacheDir)
			if err != nil {
				return nil, fmt.Errorf("loading layer %s: %w", layerPath, err)
			}
			layerDir = legacyLayer.Dir
		default:
			var err error
			rawLayer, err = config.LoadLayerRaw(resolvedPath, cacheDir)
			if err != nil {
				return nil, fmt.Errorf("loading layer %s: %w", layerPath, err)
			}
			layerDir = rawLayer.Dir
		}

		if verbose {
			fmt.Printf("  %s %s\n",
				layerStyle.Render(fmt.Sprintf("[%d/%d]", i+1, len(target.Layers))),
				layerPath)
		}

		ctx.CurrentLayer = layerPath

		// Handle remote layers (legacy path — no variable substitution)
		if legacyLayer != nil {
			for _, step := range legacyLayer.Steps {
				action, err := actions.Get(step.Action)
				if err != nil {
					return nil, fmt.Errorf("layer %s: %w", layerPath, err)
				}
				if verbose {
					printStep(step)
				}
				effectiveDir := legacyLayer.Dir
				if step.LayerSource != "" {
					sourceDir, err := config.ResolveSource(step.LayerSource, cacheDir)
					if err != nil {
						return nil, fmt.Errorf("layer %s (%s): resolving source: %w", layerPath, step.Action, err)
					}
					if step.LayerScript != "" || step.LayerScriptPath != "" {
						if err := runLayerScript(step, legacyLayer.Dir, sourceDir); err != nil {
							return nil, fmt.Errorf("layer %s (%s): layer script: %w", layerPath, step.Action, err)
						}
					}
					effectiveDir = sourceDir
				}
				if err := action.Execute(step, effectiveDir, ctx); err != nil {
					return nil, fmt.Errorf("layer %s (%s): %w", layerPath, step.Action, err)
				}
			}
			continue
		}

		// Variable scoping: validate imports, apply defaults from layer vars
		if err := validateImports(rawLayer.Imports, vars, layerPath); err != nil {
			return nil, err
		}

		// Copy vars for layer-scoped mutations
		layerVars := make(map[string]string, len(vars))
		for k, v := range vars {
			layerVars[k] = v
		}

		// Apply layer vars as defaults (don't overwrite existing values)
		for k, v := range rawLayer.Vars {
			if _, exists := layerVars[k]; !exists {
				layerVars[k] = v
			}
		}

		// Process each step with variable substitution
		for _, stepNode := range rawLayer.StepNodes {
			// Deep copy the node so substitution doesn't mutate the original
			nodeCopy := config.DeepCopyNode(stepNode)

			// Substitute ${{ var }} references
			if err := config.SubstituteVars(nodeCopy, layerVars); err != nil {
				return nil, fmt.Errorf("layer %s: %w", layerPath, err)
			}

			// Decode the substituted node into a typed Step
			step, err := config.DecodeStep(nodeCopy)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", layerPath, err)
			}

			if verbose {
				printStep(step)
			}

			effectiveLayerDir := layerDir
			if step.LayerSource != "" {
				sourceDir, err := config.ResolveSource(step.LayerSource, cacheDir)
				if err != nil {
					return nil, fmt.Errorf("layer %s (%s): resolving source: %w",
						layerPath, step.Action, err)
				}

				// Run layer build script if specified
				if step.LayerScript != "" || step.LayerScriptPath != "" {
					if err := runLayerScript(step, layerDir, sourceDir); err != nil {
						return nil, fmt.Errorf("layer %s (%s): layer script: %w",
							layerPath, step.Action, err)
					}
				}

				effectiveLayerDir = sourceDir
			}

			// Handle layer-run: execute on host, capture variable output
			if step.Action == "layer-run" {
				if ctx.DryRun {
					continue
				}
				if err := b.executeLayerRun(step, effectiveLayerDir, layerVars, ctx.Env); err != nil {
					return nil, fmt.Errorf("layer %s (layer-run): %w", layerPath, err)
				}
				continue
			}

			action, err := actions.Get(step.Action)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", layerPath, err)
			}

			if err := action.Execute(step, effectiveLayerDir, ctx); err != nil {
				return nil, fmt.Errorf("layer %s (%s): %w", layerPath, step.Action, err)
			}
		}

		// Export: if exports declared, only those vars propagate to outer scope
		if len(rawLayer.Exports) > 0 {
			for _, name := range rawLayer.Exports {
				if v, ok := layerVars[name]; ok {
					vars[name] = v
				}
			}
		} else {
			// No exports — all layerVars propagate
			for k, v := range layerVars {
				vars[k] = v
			}
		}
	}

	// Store final resolved variables in the build context
	if len(vars) > 0 {
		ctx.Vars = vars
	}

	// Print collection warnings
	for _, w := range ctx.Warnings {
		fmt.Printf("  warning: %s\n", w)
	}

	if verbose {
		fmt.Println()
	}
	return ctx, nil
}

// labelSuffix returns a dim-styled label suffix for phase output lines.
// Returns empty string if label is empty.
func labelSuffix(label string) string {
	if label == "" {
		return ""
	}
	return "  " + dimStyle.Render(label)
}

// printStep prints a step's action and optional label for verbose output.
func printStep(step config.Step) {
	if step.Label != "" {
		fmt.Printf("        %s %s\n", stepStyle.Render(step.Action), dimStyle.Render(step.Label))
	} else {
		fmt.Printf("        %s\n", stepStyle.Render(step.Action))
	}
}

// validateImports checks that all imported variables exist in the current scope.
func validateImports(imports []string, vars map[string]string, layerPath string) error {
	for _, name := range imports {
		if _, ok := vars[name]; !ok {
			return fmt.Errorf("layer %s: imported variable %q is not defined", layerPath, name)
		}
	}
	return nil
}

// substituteString replaces ${{ var }} references in a single string.
func substituteString(s string, vars map[string]string) (string, error) {
	if !strings.Contains(s, "${{") {
		return s, nil
	}
	var firstErr error
	result := config.VarPattern().ReplaceAllStringFunc(s, func(match string) string {
		sub := config.VarPattern().FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		val, ok := vars[sub[1]]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("undefined variable: %s", sub[1])
			}
			return match
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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

	return nil
}

// execute runs each build phase in order against the resolved context,
// using overlayfs layers for caching. Unchanged phases are skipped.
// After all phases, the merged tree is packaged into partition images.
func (b *Builder) execute(ctx *actions.BuildContext, buildDir string, overlay *OverlayManager) error {
	// Clean up any stale mounts and loops from a previous interrupted build
	// so we don't fail on locked resources. Scoped to buildDir only — does
	// not touch device mappers from other commands (e.g. QEMU).
	CleanupMounts(buildDir)
	cleanupLoops(buildDir)

	// Ensure mounts and loops are cleaned up when we exit, even on error.
	defer func() {
		CleanupMounts(buildDir)
		cleanupLoops(buildDir)
	}()

	// Ensure vendored dependencies are available
	if err := EnsureDeps("build"); err != nil {
		return fmt.Errorf("dependencies: %w", err)
	}

	// Initialize overlay directories
	if err := overlay.Init(); err != nil {
		return fmt.Errorf("initializing overlay: %w", err)
	}

	// Load cache manifest
	manifest, err := LoadManifest(overlay.CacheDir())
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Reject caches from a newer version of starforge
	if manifest.Version > CacheVersion && len(manifest.Phases) > 0 {
		return fmt.Errorf("cache was created by a newer version of starforge (cache version %d, this binary supports %d) — upgrade starforge or run with --clean", manifest.Version, CacheVersion)
	}

	// Clean cache if it was created with an older/incompatible format
	if manifest.Version < CacheVersion && len(manifest.Phases) > 0 {
		fmt.Printf("  Cache version mismatch (have %d, need %d), cleaning...\n", manifest.Version, CacheVersion)
		if err := overlay.CleanCache(); err != nil {
			return fmt.Errorf("cleaning incompatible cache: %w", err)
		}
		if err := overlay.Init(); err != nil {
			return fmt.Errorf("reinitializing overlay: %w", err)
		}
		manifest = &Manifest{Version: CacheVersion, Phases: make(map[string]PhaseEntry)}
		fmt.Println()
	}

	// Set version for new caches
	manifest.Version = CacheVersion

	fmt.Println(headerStyle.Render("Executing build phases"))
	fmt.Println()

	// Deduplicate packages
	if len(ctx.Packages) > 0 {
		seen := make(map[string]bool)
		var unique []string
		for _, pkg := range ctx.Packages {
			if !seen[pkg] {
				seen[pkg] = true
				unique = append(unique, pkg)
			}
		}
		ctx.Packages = unique
	}

	// Execute each phase with cache checking
	phaseFuncs := []func(*actions.BuildContext, string) error{
		b.phasePreinstall,  // 0
		b.phasePackages,    // 1
		b.phaseSysconfig,   // 2
		b.phaseUsers,       // 3
		b.phaseFiles,       // 4
		b.phasePermissions, // 5
		b.phaseServices,    // 6
		b.phaseBoot,        // 7
		b.phaseScripts,     // 8
	}

	for i, phaseFn := range phaseFuncs {
		phaseName := PhaseNames[i]

		// Compute input hash
		hash, err := HashPhase(i, ctx)
		if err != nil {
			return fmt.Errorf("hashing phase %s: %w", phaseName, err)
		}

		// Check cache
		if IsPhaseCached(overlay.CacheDir(), i, hash, manifest) {
			fmt.Printf("  %s %s\n", phaseStyle.Render(phaseName), cachedStyle.Render("cached"))
			continue
		}

		// Invalidate this and all subsequent phases
		if err := InvalidateFrom(overlay.CacheDir(), i, manifest); err != nil {
			return fmt.Errorf("invalidating from phase %s: %w", phaseName, err)
		}

		// Mount overlay for this phase
		if err := overlay.MountPhase(i); err != nil {
			return fmt.Errorf("mounting overlay for %s: %w", phaseName, err)
		}

		fmt.Printf("  %s\n", phaseStyle.Render(phaseName))

		// Execute the phase into the merged directory
		if err := phaseFn(ctx, overlay.MergedDir()); err != nil {
			overlay.Unmount()
			return fmt.Errorf("phase %s: %w", phaseName, err)
		}

		// Commit: unmount and save hash
		if err := overlay.CommitPhase(i, hash, manifest); err != nil {
			return fmt.Errorf("committing phase %s: %w", phaseName, err)
		}
	}

	// Packaging: mount all layers as read-only merged view
	mergedDir, err := overlay.MountMerged()
	if err != nil {
		return fmt.Errorf("mounting merged overlay: %w", err)
	}
	defer overlay.Unmount()

	// Package into partition images
	if err := PackageToImages(mergedDir, ctx.Partitions, buildDir, PackageOps{
		Ownerships:  ctx.FileOwnerships,
		Permissions: ctx.FilePermissions,
	}); err != nil {
		return fmt.Errorf("packaging: %w", err)
	}

	// Bundle installer payloads, server, and client into the partition images
	if err := b.bundleInstaller(ctx, buildDir); err != nil {
		return fmt.Errorf("installer bundling: %w", err)
	}

	// Invalidate named overlays since partition images have been regenerated
	if err := InvalidateOverlays(buildDir); err != nil {
		return fmt.Errorf("invalidating overlays: %w", err)
	}

	fmt.Println()
	return nil
}

// --- Individual phase implementations ---

func (b *Builder) phasePreinstall(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Keymap == "" {
		return nil
	}
	// Write vconsole.conf before pacstrap so mkinitcpio's sd-vconsole hook
	// finds it when building the initramfs during linux package installation.
	fmt.Printf("    vconsole.conf: KEYMAP=%s\n", ctx.Keymap)
	if err := os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755); err != nil {
		return fmt.Errorf("creating etc directory: %w", err)
	}
	return writeFile(filepath.Join(rootfs, "etc/vconsole.conf"), fmt.Sprintf("KEYMAP=%s\n", ctx.Keymap))
}

func (b *Builder) phasePackages(ctx *actions.BuildContext, rootfs string) error {
	if len(ctx.Packages) == 0 {
		return nil
	}
	fmt.Printf("    pacstrap %s\n", dimStyle.Render(strings.Join(ctx.Packages, ", ")))
	args := append([]string{"-K", rootfs}, ctx.Packages...)
	if err := run("pacstrap", args...); err != nil {
		return err
	}

	// Initialize and populate the pacman keyring so the installed system
	// can verify package signatures without manual key imports.
	fmt.Printf("    pacman-key %s\n", dimStyle.Render("--init, --populate archlinux"))
	if err := run("arch-chroot", rootfs, "pacman-key", "--init"); err != nil {
		return fmt.Errorf("pacman-key --init: %w", err)
	}
	if err := run("arch-chroot", rootfs, "pacman-key", "--populate", "archlinux"); err != nil {
		return fmt.Errorf("pacman-key --populate: %w", err)
	}
	return nil
}

func (b *Builder) phaseSysconfig(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Hostname != "" {
		fmt.Printf("    hostname: %s\n", ctx.Hostname)
		if err := writeFile(filepath.Join(rootfs, "etc/hostname"), ctx.Hostname+"\n"); err != nil {
			return fmt.Errorf("writing hostname: %w", err)
		}
	}

	if ctx.Locale != "" || len(ctx.Locales) > 0 {
		if ctx.Locale != "" {
			fmt.Printf("    locale:   %s\n", ctx.Locale)
			if err := writeFile(filepath.Join(rootfs, "etc/locale.conf"), fmt.Sprintf("LANG=%s\n", ctx.Locale)); err != nil {
				return fmt.Errorf("writing locale.conf: %w", err)
			}
		}

		// Collect all locales: primary (auto-included) + explicit list, deduplicated
		seen := make(map[string]bool)
		var allLocales []string
		if ctx.Locale != "" {
			seen[ctx.Locale] = true
			allLocales = append(allLocales, ctx.Locale)
		}
		for _, loc := range ctx.Locales {
			if !seen[loc] {
				seen[loc] = true
				allLocales = append(allLocales, loc)
			}
		}

		localeGen := filepath.Join(rootfs, "etc/locale.gen")
		for _, loc := range allLocales {
			fmt.Printf("    locale-gen: %s\n", loc)
			if err := appendFile(localeGen, fmt.Sprintf("%s UTF-8\n", loc)); err != nil {
				return fmt.Errorf("writing locale.gen: %w", err)
			}
		}
		if err := chrootRun(rootfs, "locale-gen"); err != nil {
			return fmt.Errorf("locale-gen: %w", err)
		}
	}

	if ctx.Timezone != "" {
		fmt.Printf("    timezone: %s\n", ctx.Timezone)
		tzLink := filepath.Join(rootfs, "etc/localtime")
		os.Remove(tzLink)
		if err := os.Symlink(filepath.Join("/usr/share/zoneinfo", ctx.Timezone), tzLink); err != nil {
			return fmt.Errorf("setting timezone: %w", err)
		}
	}

	if ctx.Keymap != "" {
		fmt.Printf("    keymap:   %s\n", ctx.Keymap)
	}

	return nil
}

func (b *Builder) phaseUsers(ctx *actions.BuildContext, rootfs string) error {
	// Create explicit groups first (system-group action)
	for _, group := range ctx.Groups {
		args := []string{"groupadd", "-f"}
		if group.System {
			args = append(args, "-r")
		}
		if group.GID != 0 {
			args = append(args, "-g", fmt.Sprintf("%d", group.GID))
		}
		args = append(args, group.Name)
		fmt.Printf("    group: %s\n", group.Name)
		chrootRun(rootfs, args...)
	}

	for _, user := range ctx.Users {
		groups := ""
		if len(user.Groups) > 0 {
			groups = fmt.Sprintf(" (%s)", strings.Join(user.Groups, ", "))
		}
		label := user.Name
		if user.System {
			label += " (system)"
		}
		fmt.Printf("    %s%s\n", label, groups)

		// Create implicit groups from user group lists
		for _, group := range user.Groups {
			chrootRun(rootfs, "groupadd", "-f", group)
		}

		args := []string{"useradd"}
		if user.System {
			args = append(args, "-r", "-M") // system user, no home directory
		} else {
			args = append(args, "-m") // create home directory
		}
		if user.Shell != "" {
			args = append(args, "-s", user.Shell)
		}
		if user.UID != 0 {
			args = append(args, "-u", fmt.Sprintf("%d", user.UID))
		}
		if len(user.Groups) > 0 {
			args = append(args, "-G", strings.Join(user.Groups, ","))
		}
		args = append(args, user.Name)

		if err := chrootRun(rootfs, args...); err != nil {
			return fmt.Errorf("creating user %s: %w", user.Name, err)
		}

		if user.NoPassword {
			if err := chrootRun(rootfs, "passwd", "-d", user.Name); err != nil {
				return fmt.Errorf("removing password for %s: %w", user.Name, err)
			}
		} else if user.Password != "" {
			cmd := exec.Command(resolveBin("arch-chroot"), rootfs, "chpasswd")
			cmd.Env = vendorEnv()
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", user.Name, user.Password))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("setting password for %s: %w", user.Name, err)
			}
		}
	}
	return nil
}

func (b *Builder) phaseFiles(ctx *actions.BuildContext, rootfs string) error {
	// 1. Create directories (file-mkdir)
	for _, m := range ctx.FileMkdirs {
		target := filepath.Join(rootfs, m.Path)
		fmt.Printf("    mkdir %s%s\n", m.Path, labelSuffix(m.Label))
		mode := parseMode(m.Mode, 0o755)
		if err := os.MkdirAll(target, mode); err != nil {
			return fmt.Errorf("mkdir %s: %w", m.Path, err)
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", m.Path, err)
		}
		if m.Owner != "" || m.Group != "" {
			ownership := fmt.Sprintf("%s:%s", m.Owner, m.Group)
			if err := chrootRun(rootfs, "chown", ownership, m.Path); err != nil {
				return fmt.Errorf("chown %s: %w", m.Path, err)
			}
		}
	}

	// 2. Layer copies (file-create with dir layer_path, systemd-unit, systemd-config)
	for _, cp := range ctx.LayerCopies {
		src := filepath.Join(cp.LayerDir, cp.FromPath)
		dest := filepath.Join(rootfs, cp.ToPath)
		fmt.Printf("    %s -> %s%s\n", cp.FromPath, cp.ToPath, labelSuffix(cp.Label))

		srcInfo, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", cp.FromPath, err)
		}

		if srcInfo.IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", cp.ToPath, err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", cp.ToPath, err)
			}
		}

		fs := filesystemForPath(cp.ToPath, ctx.Partitions)
		if err := copyForFilesystem(src, dest, fs); err != nil {
			return fmt.Errorf("copying %s to %s: %w", cp.FromPath, cp.ToPath, err)
		}
	}

	// 3. File creates (file-create with content or file layer_path)
	for _, fc := range ctx.FileCreates {
		fmt.Printf("    create %s%s\n", fc.Path, labelSuffix(fc.Label))
		target := filepath.Join(rootfs, fc.Path)

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", fc.Path, err)
		}
		mode := parseMode(fc.Mode, 0o644)
		if err := os.WriteFile(target, []byte(fc.Content), mode); err != nil {
			return fmt.Errorf("writing %s: %w", fc.Path, err)
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", fc.Path, err)
		}
	}

	// 4. File edits (file-edit)
	for _, fe := range ctx.FileEdits {
		fmt.Printf("    edit %s%s\n", fe.Path, labelSuffix(fe.Label))
		if err := applyFileEdit(rootfs, fe); err != nil {
			return err
		}
	}

	// 5. Internal copies (file-copy, within target)
	for _, ic := range ctx.FileCopies {
		fmt.Printf("    copy %s -> %s%s\n", ic.FromPath, ic.ToPath, labelSuffix(ic.Label))
		src := filepath.Join(rootfs, ic.FromPath)
		dest := filepath.Join(rootfs, ic.ToPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", ic.ToPath, err)
		}
		if err := run("cp", "-rT", src, dest); err != nil {
			return fmt.Errorf("copying %s to %s: %w", ic.FromPath, ic.ToPath, err)
		}
	}

	// 6. Moves (file-move)
	for _, mv := range ctx.FileMoves {
		fmt.Printf("    move %s -> %s%s\n", mv.FromPath, mv.ToPath, labelSuffix(mv.Label))
		src := filepath.Join(rootfs, mv.FromPath)
		dest := filepath.Join(rootfs, mv.ToPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", mv.ToPath, err)
		}
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("moving %s to %s: %w", mv.FromPath, mv.ToPath, err)
		}
	}

	// 7. Links (file-link)
	for _, ln := range ctx.FileLinks {
		fmt.Printf("    %s %s -> %s%s\n", ln.Type, ln.ToPath, ln.FromPath, labelSuffix(ln.Label))
		dest := filepath.Join(rootfs, ln.ToPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", ln.ToPath, err)
		}
		os.Remove(dest) // remove existing link/file if present
		switch ln.Type {
		case "hard":
			src := filepath.Join(rootfs, ln.FromPath)
			if err := os.Link(src, dest); err != nil {
				return fmt.Errorf("hard link %s -> %s: %w", ln.ToPath, ln.FromPath, err)
			}
		default:
			if err := os.Symlink(ln.FromPath, dest); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", ln.ToPath, ln.FromPath, err)
			}
		}
	}

	// 8. Deletes (file-delete, runs last)
	for _, r := range ctx.FileDeletes {
		fmt.Printf("    delete %s%s\n", r.Path, labelSuffix(r.Label))
		target := filepath.Join(rootfs, r.Path)
		if r.Recursive {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("removing %s: %w", r.Path, err)
			}
		} else {
			if err := os.Remove(target); err != nil {
				return fmt.Errorf("removing %s: %w", r.Path, err)
			}
		}
	}

	return nil
}

// applyFileEdit reads a file, applies an insert/truncate operation, and writes it back.
func applyFileEdit(rootfs string, edit actions.FileEditOp) error {
	path := filepath.Join(rootfs, edit.Path)

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file-edit %s: %w", edit.Path, err)
	}
	mode := info.Mode().Perm()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("file-edit %s: %w", edit.Path, err)
	}
	content := string(data)

	switch {
	case edit.Truncate != "":
		content = actions.TruncatePattern(content, edit.Pattern, edit.Truncate, edit.Match)
	case edit.Insert == "append":
		content += edit.Content
	case edit.Insert == "prepend":
		content = edit.Content + content
	case edit.Insert == "before" || edit.Insert == "after":
		content = actions.InsertPattern(content, edit.Pattern, edit.Content, edit.Insert, edit.Match)
	}

	return os.WriteFile(path, []byte(content), mode)
}

// parseMode parses an octal mode string, returning defaultMode on failure.
func parseMode(s string, defaultMode os.FileMode) os.FileMode {
	if s == "" {
		return defaultMode
	}
	var mode uint32
	if _, err := fmt.Sscanf(s, "%o", &mode); err != nil {
		return defaultMode
	}
	return os.FileMode(mode)
}

// filesystemForPath returns the filesystem type for a path by finding the
// most specific matching partition mount point. Defaults to "ext4".
func filesystemForPath(path string, parts []actions.PartitionDef) string {
	best := ""
	fs := "ext4"
	cleanPath := filepath.Clean(path)
	for _, p := range parts {
		mp := filepath.Clean(p.MountPoint)
		if mp == "/" || cleanPath == mp || strings.HasPrefix(cleanPath, mp+"/") {
			if len(mp) > len(best) {
				best = mp
				fs = p.Filesystem
			}
		}
	}
	return fs
}

func (b *Builder) phasePermissions(ctx *actions.BuildContext, rootfs string) error {
	// Ownership changes (file-ownership / chown)
	for _, own := range ctx.FileOwnerships {
		target := filepath.Join(rootfs, own.Path)
		os.MkdirAll(target, 0o755)

		ownership := fmt.Sprintf("%s:%s", own.Owner, own.Group)
		fmt.Printf("    chown %s %s%s\n", ownership, own.Path, labelSuffix(own.Label))

		args := []string{ownership, own.Path}
		if own.Recursive {
			args = []string{"-R", ownership, own.Path}
		}
		if err := chrootRun(rootfs, append([]string{"chown"}, args...)...); err != nil {
			return fmt.Errorf("chown %s: %w", own.Path, err)
		}
	}

	// Mode changes (file-permissions / chmod)
	for _, perm := range ctx.FilePermissions {
		target := filepath.Join(rootfs, perm.Path)
		os.MkdirAll(target, 0o755)

		fmt.Printf("    chmod %s %s%s\n", perm.Mode, perm.Path, labelSuffix(perm.Label))

		args := []string{perm.Mode, target}
		if perm.Recursive {
			args = []string{"-R", perm.Mode, target}
		}
		if err := run("chmod", args...); err != nil {
			return fmt.Errorf("chmod %s: %w", perm.Path, err)
		}
	}

	return nil
}

func (b *Builder) phaseServices(ctx *actions.BuildContext, rootfs string) error {
	for _, svc := range ctx.Services.Mask {
		fmt.Printf("    mask:    %s\n", svc)
		if err := chrootRun(rootfs, "systemctl", "mask", svc); err != nil {
			return fmt.Errorf("masking %s: %w", svc, err)
		}
	}

	for _, svc := range ctx.Services.Enable {
		fmt.Printf("    enable:  %s\n", svc)
		if err := chrootRun(rootfs, "systemctl", "enable", svc); err != nil {
			return fmt.Errorf("enabling %s: %w", svc, err)
		}
	}

	for _, svc := range ctx.Services.Disable {
		fmt.Printf("    disable: %s\n", svc)
		if err := chrootRun(rootfs, "systemctl", "disable", svc); err != nil {
			return fmt.Errorf("disabling %s: %w", svc, err)
		}
	}

	// User-level enable: parse [Install] sections and create symlinks
	for _, op := range ctx.Services.UserEnable {
		fmt.Printf("    enable:  %s (user: %s)\n", op.Service, op.User)
		if err := enableUserUnit(rootfs, op.User, op.Service); err != nil {
			return fmt.Errorf("enabling user unit %s for %s: %w", op.Service, op.User, err)
		}
	}

	// User-level disable: remove symlinks
	for _, op := range ctx.Services.UserDisable {
		fmt.Printf("    disable: %s (user: %s)\n", op.Service, op.User)
		if err := disableUserUnit(rootfs, op.User, op.Service); err != nil {
			return fmt.Errorf("disabling user unit %s for %s: %w", op.Service, op.User, err)
		}
	}

	if ctx.DefaultTarget != "" {
		fmt.Printf("    default: %s\n", ctx.DefaultTarget)
		if err := chrootRun(rootfs, "systemctl", "set-default", ctx.DefaultTarget); err != nil {
			return fmt.Errorf("setting default target %s: %w", ctx.DefaultTarget, err)
		}
	}

	return nil
}

func (b *Builder) phaseBoot(ctx *actions.BuildContext, rootfs string) error {
	if ctx.Boot == nil {
		return nil
	}

	loaderDir := filepath.Join(rootfs, "boot/loader")
	entriesDir := filepath.Join(loaderDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return fmt.Errorf("creating boot loader directories: %w", err)
	}

	loader := fmt.Sprintf("default %s\ntimeout %d\neditor %s\n",
		ctx.Boot.Loader.Default,
		ctx.Boot.Loader.Timeout,
		boolToNo(ctx.Boot.Loader.Editor))
	fmt.Printf("    loader.conf (default=%s, timeout=%d)\n",
		ctx.Boot.Loader.Default, ctx.Boot.Loader.Timeout)
	if err := writeFile(filepath.Join(loaderDir, "loader.conf"), loader); err != nil {
		return fmt.Errorf("writing loader.conf: %w", err)
	}

	for _, entry := range ctx.Boot.Entries {
		fmt.Printf("    entry: %s (%s)\n", entry.Name, entry.Title)
		content := fmt.Sprintf("title   %s\nlinux   %s\ninitrd  %s\noptions %s\n",
			entry.Title, entry.Linux, entry.Initrd, entry.Options)
		if err := writeFile(filepath.Join(entriesDir, entry.Name), content); err != nil {
			return fmt.Errorf("writing boot entry %s: %w", entry.Name, err)
		}
	}
	return nil
}

func (b *Builder) phaseScripts(ctx *actions.BuildContext, rootfs string) error {
	for _, script := range ctx.Scripts {
		// Determine display label
		label := script.Label
		if label == "" {
			label = script.Script
		}
		if label == "" {
			label = "(inline)"
		}
		if script.User != "" {
			fmt.Printf("    %s  %s\n", label, dimStyle.Render("user: "+script.User))
		} else {
			fmt.Printf("    %s\n", label)
		}

		// Use /var/tmp instead of /tmp — arch-chroot mounts a tmpfs on /tmp
		// which would hide any files we write to the overlay's /tmp.
		if err := os.MkdirAll(filepath.Join(rootfs, "var", "tmp"), 0o755); err != nil {
			return fmt.Errorf("creating var/tmp dir: %w", err)
		}

		// Build script prelude with sf_set/sf_get + variable declarations
		prelude := buildScriptPrelude(ctx.Vars)

		var tmpScript, chrootScriptPath string

		if script.Script != "" {
			// File-based: read from layer dir, prepend prelude, write to chroot
			scriptPath := filepath.Join(script.LayerDir, script.Script)
			scriptData, err := os.ReadFile(scriptPath)
			if err != nil {
				return fmt.Errorf("reading script %s: %w", script.Script, err)
			}
			content := injectPrelude(string(scriptData), prelude)
			tmpScript = filepath.Join(rootfs, "var/tmp", filepath.Base(script.Script))
			if err := os.WriteFile(tmpScript, []byte(content), 0o755); err != nil {
				return fmt.Errorf("writing script %s: %w", script.Script, err)
			}
			chrootScriptPath = filepath.Join("/var/tmp", filepath.Base(script.Script))
		} else {
			// Inline: write prelude + content to temp file
			content := injectPrelude(script.Content, prelude)
			tmpScript = filepath.Join(rootfs, "var/tmp", "starforge-inline.sh")
			if err := os.WriteFile(tmpScript, []byte(content), 0o755); err != nil {
				return fmt.Errorf("writing inline script: %w", err)
			}
			chrootScriptPath = "/var/tmp/starforge-inline.sh"
		}

		if err := run("chmod", "+x", tmpScript); err != nil {
			return fmt.Errorf("making script executable: %w", err)
		}

		// Merge env: target-level + step-level (step overrides target)
		env := mergeScriptEnv(ctx.Env, script.Env)

		var runErr error
		if script.User != "" {
			runErr = chrootRunWithEnv(rootfs, env, "su", "-", script.User, "-s", "/bin/bash", "-c", chrootScriptPath)
		} else {
			runErr = chrootRunWithEnv(rootfs, env, chrootScriptPath)
		}
		os.Remove(tmpScript)
		if runErr != nil {
			return fmt.Errorf("running script %s: %w", label, runErr)
		}
	}
	return nil
}

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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// chrootRun executes a command inside the rootfs using arch-chroot.
func chrootRun(rootfs string, args ...string) error {
	cmdArgs := append([]string{rootfs}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// writeFile writes content to a file, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// appendFile appends content to a file, creating it if it doesn't exist.
func appendFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// CopyFile copies a file from src to dest using streaming I/O.
func CopyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// boolToNo returns "no" for false, "yes" for true.
func boolToNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Chroot enters the built filesystem interactively, or runs a command inside it.
// If args is empty, an interactive shell is started.
//
// When overlayName is set, partition images from a named overlay are loop-mounted
// and changes persist across sessions. When empty, the ephemeral overlayfs
// behavior is used (changes discarded on exit).
func (b *Builder) Chroot(targetName string, args []string, overlayName string, parts []actions.PartitionDef) error {
	buildDir := b.project.TargetBuildDir(targetName)

	if overlayName != "" {
		return b.chrootOverlay(targetName, buildDir, args, overlayName, parts)
	}

	overlay := NewOverlayManager(buildDir)
	manifest, err := LoadManifest(overlay.CacheDir())
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Check that at least one phase has been built
	if len(manifest.Phases) == 0 {
		return fmt.Errorf("target %q has not been built yet — run 'starforge build %s' first", targetName, targetName)
	}

	fmt.Println(headerStyle.Render(fmt.Sprintf("Entering chroot: %s", targetName)))

	// Clean up any stale mounts from a previous session
	CleanupMounts(buildDir)

	// Mount all cached layers as a writable overlay (changes are discarded on unmount)
	mergedDir, err := overlay.MountMergedWritable()
	if err != nil {
		return fmt.Errorf("mounting overlay: %w", err)
	}
	defer func() {
		overlay.Unmount()
		overlay.CleanChroot()
	}()

	cmdArgs := append([]string{mergedDir}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// chrootOverlay enters a chroot using loop-mounted partition images from a named overlay.
func (b *Builder) chrootOverlay(targetName, buildDir string, args []string, overlayName string, parts []actions.PartitionDef) error {
	fmt.Println(headerStyle.Render(fmt.Sprintf("Entering chroot: %s (overlay: %s)", targetName, overlayName)))

	overlayDir, err := EnsureNamedOverlay(buildDir, overlayName, parts)
	if err != nil {
		return fmt.Errorf("named overlay: %w", err)
	}

	// Build partition mounts from overlay images (skip swap)
	var mounts []PartitionMount
	for _, part := range parts {
		if part.Filesystem == "swap" {
			continue
		}
		mounts = append(mounts, PartitionMount{
			Source:     filepath.Join(overlayDir, fmt.Sprintf("%s.img", part.Name)),
			MountPoint: part.MountPoint,
			Loop:       true,
		})
	}

	if len(mounts) == 0 {
		return fmt.Errorf("no mountable partitions found")
	}

	// Create temp dir as mount root
	rootfs, err := os.MkdirTemp("", "starforge-chroot-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(rootfs)

	mt := NewMountTable(rootfs)
	if err := mt.MountAll(mounts); err != nil {
		mt.Unmount()
		return fmt.Errorf("mounting partitions: %w", err)
	}
	defer mt.Unmount()

	cmdArgs := append([]string{rootfs}, args...)
	cmd := exec.Command(resolveBin("arch-chroot"), cmdArgs...)
	cmd.Env = vendorEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// enableUserUnit creates symlinks for a user-level systemd unit, replicating
// what `systemctl --user enable` does: parse the unit's [Install] section
// and create .wants/.requires symlinks + aliases.
func enableUserUnit(rootfs, user, service string) error {
	userDir := filepath.Join(rootfs, "home", user, ".config/systemd/user")
	unitPath := findUserUnit(rootfs, user, service)
	if unitPath == "" {
		return fmt.Errorf("unit file not found: %s", service)
	}

	install, err := parseInstallSection(unitPath)
	if err != nil {
		return fmt.Errorf("parsing [Install] for %s: %w", service, err)
	}

	// Determine symlink target (what the symlink points to)
	linkTarget := unitPath
	// If in user config dir, use a path relative to the user's systemd dir
	if strings.HasPrefix(unitPath, userDir) {
		// Point to the unit in the same directory
		linkTarget = filepath.Join("/home", user, ".config/systemd/user", service)
	} else {
		// System-provided unit — use absolute path
		rel, _ := filepath.Rel(rootfs, unitPath)
		linkTarget = "/" + rel
	}

	// Create WantedBy symlinks
	for _, target := range install.WantedBy {
		wantsDir := filepath.Join(userDir, target+".wants")
		if err := os.MkdirAll(wantsDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", wantsDir, err)
		}
		link := filepath.Join(wantsDir, service)
		os.Remove(link) // remove existing
		if err := os.Symlink(linkTarget, link); err != nil {
			return fmt.Errorf("creating symlink %s: %w", link, err)
		}
	}

	// Create RequiredBy symlinks
	for _, target := range install.RequiredBy {
		requiresDir := filepath.Join(userDir, target+".requires")
		if err := os.MkdirAll(requiresDir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", requiresDir, err)
		}
		link := filepath.Join(requiresDir, service)
		os.Remove(link)
		if err := os.Symlink(linkTarget, link); err != nil {
			return fmt.Errorf("creating symlink %s: %w", link, err)
		}
	}

	// Create Alias symlinks
	for _, alias := range install.Alias {
		link := filepath.Join(userDir, alias)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return err
		}
		os.Remove(link)
		if err := os.Symlink(linkTarget, link); err != nil {
			return fmt.Errorf("creating alias %s: %w", link, err)
		}
	}

	// Create Also enables (recursive)
	for _, also := range install.Also {
		if err := enableUserUnit(rootfs, user, also); err != nil {
			return fmt.Errorf("enabling Also=%s: %w", also, err)
		}
	}

	return nil
}

// disableUserUnit removes symlinks for a user-level systemd unit.
func disableUserUnit(rootfs, user, service string) error {
	userDir := filepath.Join(rootfs, "home", user, ".config/systemd/user")
	unitPath := findUserUnit(rootfs, user, service)
	if unitPath == "" {
		return fmt.Errorf("unit file not found: %s", service)
	}

	install, err := parseInstallSection(unitPath)
	if err != nil {
		return fmt.Errorf("parsing [Install] for %s: %w", service, err)
	}

	for _, target := range install.WantedBy {
		os.Remove(filepath.Join(userDir, target+".wants", service))
	}
	for _, target := range install.RequiredBy {
		os.Remove(filepath.Join(userDir, target+".requires", service))
	}
	for _, alias := range install.Alias {
		os.Remove(filepath.Join(userDir, alias))
	}

	return nil
}

// findUserUnit locates a user unit file, checking user config dir first,
// then system user unit directories.
func findUserUnit(rootfs, user, service string) string {
	candidates := []string{
		filepath.Join(rootfs, "home", user, ".config/systemd/user", service),
		filepath.Join(rootfs, "usr/lib/systemd/user", service),
		filepath.Join(rootfs, "etc/systemd/user", service),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// installSection holds parsed [Install] directives from a systemd unit file.
type installSection struct {
	WantedBy   []string
	RequiredBy []string
	Alias      []string
	Also       []string
}

// parseInstallSection reads a systemd unit file and extracts [Install] directives.
func parseInstallSection(path string) (installSection, error) {
	f, err := os.Open(path)
	if err != nil {
		return installSection{}, err
	}
	defer f.Close()

	var result installSection
	inInstall := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inInstall = line == "[Install]"
			continue
		}
		if !inInstall {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		// Split on whitespace — systemd allows space-separated lists
		values := strings.Fields(strings.TrimSpace(value))
		switch key {
		case "WantedBy":
			result.WantedBy = append(result.WantedBy, values...)
		case "RequiredBy":
			result.RequiredBy = append(result.RequiredBy, values...)
		case "Alias":
			result.Alias = append(result.Alias, values...)
		case "Also":
			result.Also = append(result.Also, values...)
		}
	}
	return result, scanner.Err()
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

