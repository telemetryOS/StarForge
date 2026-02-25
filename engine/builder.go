package engine

import (
	"fmt"
	"os"
	"path/filepath"
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

	// Tee all output to build.log (non-fatal if it fails)
	defer wrapBuildLog(buildDir)()

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

	// Save build result so EnsurePackaged can package without re-collecting
	if err := SaveBuildResult(ctx, buildDir); err != nil {
		return fmt.Errorf("saving build result: %w", err)
	}

	fmt.Println(successStyle.Render(fmt.Sprintf("Build complete: %s", buildDir)))
	return nil
}

// EnsurePackaged guarantees that up-to-date partition images (.img files) exist
// for the given target. If all build phases are cached and packaging is marked
// complete in the manifest, this is a no-op. Otherwise it reads the saved
// build result and re-packages from the overlay layers without re-collecting.
//
// Returns the BuildContext (with partition definitions) so callers can use it
// for RunQEMU or WriteToDevice without a separate Collect call.
func (b *Builder) EnsurePackaged(targetName string) (*actions.BuildContext, error) {
	buildDir := b.project.TargetBuildDir(targetName)

	// Tee all output to build.log (non-fatal if it fails)
	defer wrapBuildLog(buildDir)()

	overlay := NewOverlayManager(buildDir)

	// Load build result saved by the last Build — this has everything we need
	// for packaging without re-running Collect (which would re-execute layer-run).
	result, err := LoadBuildResult(buildDir)
	if err != nil {
		return nil, fmt.Errorf("no build result found — run 'starforge build %s' first", targetName)
	}

	// Load manifest and check if all phases are completed
	manifest, err := LoadManifest(overlay.CacheDir())
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	for _, name := range PhaseNames {
		entry, ok := manifest.Phases[name]
		if !ok || !entry.Completed {
			return nil, fmt.Errorf("build not complete — run 'starforge build %s' first", targetName)
		}
	}

	// If packaging is marked complete and .img files exist, we're done
	if manifest.Packaging != nil && manifest.Packaging.Completed {
		allExist := true
		for _, p := range result.Partitions {
			if _, err := os.Stat(filepath.Join(buildDir, p.Name+".img")); err != nil {
				allExist = false
				break
			}
		}
		if allExist {
			fmt.Println(cachedStyle.Render("Packaging up to date"))
			return &actions.BuildContext{Partitions: result.Partitions}, nil
		}
	}

	// Need to (re-)package from saved build result
	fmt.Println(headerStyle.Render("Packaging images"))
	fmt.Println()

	CleanupMounts(buildDir)
	cleanupLoops(buildDir)
	defer func() {
		CleanupMounts(buildDir)
		cleanupLoops(buildDir)
	}()

	if err := EnsureDeps("build"); err != nil {
		return nil, fmt.Errorf("dependencies: %w", err)
	}

	if err := overlay.Init(); err != nil {
		return nil, fmt.Errorf("initializing overlay: %w", err)
	}

	mergedDir, err := overlay.MountMerged()
	if err != nil {
		return nil, fmt.Errorf("mounting merged overlay: %w", err)
	}
	defer overlay.Unmount()

	if err := PackageToImages(mergedDir, result.Partitions, buildDir, PackageOps{
		Ownerships:  result.Ownerships,
		Permissions: result.Permissions,
	}); err != nil {
		return nil, fmt.Errorf("packaging: %w", err)
	}

	// Bundle installer using a minimal BuildContext from the saved result
	ctx := &actions.BuildContext{
		Partitions:        result.Partitions,
		InstallerPayloads: result.InstallerPayloads,
		InstallerServer:   result.InstallerServer,
		InstallerClient:   result.InstallerClient,
	}
	if err := b.bundleInstaller(ctx, buildDir); err != nil {
		return nil, fmt.Errorf("installer bundling: %w", err)
	}

	if err := InvalidateOverlays(buildDir); err != nil {
		return nil, fmt.Errorf("invalidating overlays: %w", err)
	}

	// Mark packaging complete
	manifest.Packaging = &PackagingEntry{Hash: "packaged", Completed: true}
	if err := manifest.Save(overlay.CacheDir()); err != nil {
		return nil, fmt.Errorf("saving manifest: %w", err)
	}

	return ctx, nil
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

	// Initialize variable scope from target args.
	// Env var references ($NAME or ${NAME}) in values are expanded from the
	// host environment, falling back to default_env values when unset.
	vars := make(map[string]string)
	envLookup := func(key string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return target.DefaultEnv[key]
	}
	for k, v := range target.Args {
		vars[k] = os.Expand(v, envLookup)
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

	// Deduplicate packages (data normalization belongs in collection)
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


// execute runs each build phase in order against the resolved context,
// using overlayfs layers for caching. Unchanged phases are skipped.
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

	fmt.Println()
	return nil
}


