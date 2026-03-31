package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
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
		out.Info("Cleaning cache")
		if err := overlay.CleanCache(); err != nil {
			return fmt.Errorf("cleaning cache: %w", err)
		}
	}

	if err := b.execute(ctx, buildDir, overlay); err != nil {
		overlay.Unmount()
		return err
	}

	// Save build result so EnsurePackaged can package without re-collecting
	if err := SaveBuildResult(ctx, buildDir); err != nil {
		return fmt.Errorf("saving build result: %w", err)
	}

	return nil
}

// EnsureBuiltAndPackaged guarantees that a target has been fully built and
// packaged. If no build result exists or the build is incomplete, runs a
// full Build first, then packages. This is the entry point for commands
// like run and write that need images to exist without requiring a separate
// build step.
func (b *Builder) EnsureBuiltAndPackaged(targetName string) (*actions.BuildContext, error) {
	target, ok := b.project.Targets[targetName]
	if !ok {
		return nil, fmt.Errorf("target %q not found in project", targetName)
	}

	// Always run an incremental build. Build re-collects layers, hashes
	// each phase against the manifest, and only rebuilds what changed.
	// This correctly detects source file changes that EnsurePackaged alone
	// would miss (it only checks whether .img files exist on disk).
	if err := b.Build(targetName, target, false); err != nil {
		return nil, err
	}

	return b.EnsurePackaged(targetName)
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

	// Compute packaging hash including payload target manifests.
	// This ensures changes to any payload target invalidate the installer's packaging.
	ctx := buildResultToContext(result)
	packagingHash := HashPackaging(manifest, ctx, b.project)

	if manifest.Packaging != nil && manifest.Packaging.Completed && manifest.Packaging.Hash == packagingHash {
		allExist := true
		for _, p := range result.Partitions {
			if _, err := os.Stat(filepath.Join(buildDir, p.Name+".img")); err != nil {
				allExist = false
				break
			}
		}
		if allExist {
			out.Success("Packaging up to date")
			return buildResultToContext(result), nil
		}
	}

	// Need to (re-)package
	out.StartStage(StagePackage)
	packageStart := time.Now()

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

	// Bundle installer using a BuildContext from the saved result
	if err := b.bundleInstaller(ctx, buildDir); err != nil {
		return nil, fmt.Errorf("installer bundling: %w", err)
	}

	if err := InvalidateOverlays(buildDir); err != nil {
		return nil, fmt.Errorf("invalidating overlays: %w", err)
	}

	// Mark packaging complete with the content hash
	manifest.Packaging = &PackagingEntry{Hash: packagingHash, Completed: true}
	if err := manifest.Save(overlay.CacheDir()); err != nil {
		return nil, fmt.Errorf("saving manifest: %w", err)
	}

	out.EndStage(StagePackage, time.Since(packageStart))
	return ctx, nil
}

// Collect processes all layers and returns the resolved BuildContext.
// If verbose is true, prints progress to stdout.
func (b *Builder) Collect(target config.Target, verbose bool) (*actions.BuildContext, error) {
	out.StartStage(StageCollect)
	collectStart := time.Now()

	ctx := actions.NewBuildContext()
	ctx.DryRun = b.DryRun

	cacheDir := filepath.Join(b.project.BuildDir(), "cache")
	ctx.DownloadCacheDir = cacheDir

	vars, err := b.initVars(target)
	if err != nil {
		return nil, err
	}
	if err := b.resolveTargetEnv(target, vars, ctx); err != nil {
		return nil, err
	}

	for i, layerPath := range target.Layers {
		rawLayer, legacyLayer, layerDir, err := b.loadLayer(layerPath, cacheDir)
		if err != nil {
			return nil, err
		}

		layerStart := time.Now()
		if verbose {
			out.CollectLayer(i, layerPath)
		}
		ctx.CurrentLayer = layerPath

		if legacyLayer != nil {
			if err := b.collectLegacyLayer(legacyLayer, layerPath, cacheDir, verbose, ctx); err != nil {
				return nil, err
			}
		} else {
			layerVars, err := b.collectRawLayer(rawLayer, layerPath, layerDir, cacheDir, vars, verbose, ctx)
			if err != nil {
				return nil, err
			}
			propagateVars(rawLayer.Exports, layerVars, vars)
		}

		if verbose {
			out.CollectLayerDone(i, time.Since(layerStart))
		}
	}

	if len(vars) > 0 {
		ctx.Vars = vars
	}

	ctx.Packages = deduplicatePackages(ctx.Packages)

	if err := b.resolvePkgRels(ctx, verbose); err != nil {
		return nil, err
	}

	for _, w := range ctx.Warnings {
		out.Warning(w)
	}
	if verbose {
		out.Blank()
	}

	out.EndStage(StageCollect, time.Since(collectStart))
	return ctx, nil
}

// initVars builds the initial variable map from target args, expanding any
// $NAME / ${NAME} references from the host environment.
func (b *Builder) initVars(target config.Target) (map[string]string, error) {
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
	return vars, nil
}

// resolveTargetEnv resolves variable substitutions in target-level env values
// and stores the result in ctx.Env.
func (b *Builder) resolveTargetEnv(target config.Target, vars map[string]string, ctx *actions.BuildContext) error {
	if len(target.Env) == 0 {
		return nil
	}
	ctx.Env = make(map[string]string, len(target.Env))
	for k, v := range target.Env {
		resolved, err := substituteString(v, vars)
		if err != nil {
			return fmt.Errorf("target env %s: %w", k, err)
		}
		ctx.Env[k] = resolved
	}
	return nil
}

// loadLayer resolves a layer path and loads the layer config.
// Returns either a RawLayer (local/git/archive) or a legacy Layer (remote URL).
func (b *Builder) loadLayer(layerPath, cacheDir string) (rawLayer *config.RawLayer, legacyLayer *config.Layer, layerDir string, err error) {
	resolvedPath := b.project.ResolveLayerPath(layerPath)

	switch {
	case config.IsGitSource(resolvedPath) || config.IsArchiveSource(resolvedPath):
		sourceDir, fetchErr := config.ResolveSource(resolvedPath, cacheDir)
		if fetchErr != nil {
			return nil, nil, "", fmt.Errorf("fetching source layer %s: %w", layerPath, fetchErr)
		}
		rawLayer, err = config.LoadLayerRaw(sourceDir, cacheDir)
		if err != nil {
			return nil, nil, "", fmt.Errorf("loading layer %s: %w", layerPath, err)
		}
		return rawLayer, nil, rawLayer.Dir, nil

	case config.IsURL(resolvedPath):
		// Remote layers use LoadLayer — variable substitution is not supported
		mirrorDir, fetchErr := config.FetchRemoteLayer(resolvedPath, cacheDir)
		if fetchErr != nil {
			return nil, nil, "", fmt.Errorf("fetching remote layer %s: %w", layerPath, fetchErr)
		}
		legacyLayer, err = config.LoadLayer(mirrorDir, cacheDir)
		if err != nil {
			return nil, nil, "", fmt.Errorf("loading layer %s: %w", layerPath, err)
		}
		return nil, legacyLayer, legacyLayer.Dir, nil

	default:
		rawLayer, err = config.LoadLayerRaw(resolvedPath, cacheDir)
		if err != nil {
			return nil, nil, "", fmt.Errorf("loading layer %s: %w", layerPath, err)
		}
		return rawLayer, nil, rawLayer.Dir, nil
	}
}

// collectLegacyLayer executes all steps in a remote (legacy) layer.
// Legacy layers do not support variable substitution.
func (b *Builder) collectLegacyLayer(layer *config.Layer, layerPath, cacheDir string, verbose bool, ctx *actions.BuildContext) error {
	for _, step := range layer.Steps {
		action, err := actions.Get(step.Action)
		if err != nil {
			return fmt.Errorf("layer %s: %w", layerPath, err)
		}
		if verbose {
			printStep(step)
		}
		effectiveDir := layer.Dir
		if step.LayerSource != "" {
			sourceDir, err := config.ResolveSource(step.LayerSource, cacheDir)
			if err != nil {
				return fmt.Errorf("layer %s (%s): resolving source: %w", layerPath, step.Action, err)
			}
			if step.LayerScript != "" || step.LayerScriptPath != "" {
				if err := runLayerScript(step, layer.Dir, sourceDir); err != nil {
					return fmt.Errorf("layer %s (%s): layer script: %w", layerPath, step.Action, err)
				}
			}
			effectiveDir = sourceDir
		}
		if err := action.Execute(step, effectiveDir, ctx); err != nil {
			return fmt.Errorf("layer %s (%s): %w", layerPath, step.Action, err)
		}
	}
	return nil
}

// collectRawLayer processes a raw (local) layer: validates imports, applies
// defaults, substitutes variables, and executes each step.
// Returns the layer-scoped variable map so the caller can propagate exports.
func (b *Builder) collectRawLayer(rawLayer *config.RawLayer, layerPath, layerDir, cacheDir string, vars map[string]string, verbose bool, ctx *actions.BuildContext) (map[string]string, error) {
	if err := validateImports(rawLayer.Imports, vars, layerPath); err != nil {
		return nil, err
	}

	// Layer vars are scoped: start with a copy and apply this layer's defaults
	layerVars := make(map[string]string, len(vars))
	for k, v := range vars {
		layerVars[k] = v
	}
	for k, v := range rawLayer.Vars {
		if _, exists := layerVars[k]; !exists {
			layerVars[k] = v
		}
	}

	for _, stepNode := range rawLayer.StepNodes {
		nodeCopy := config.DeepCopyNode(stepNode)

		if err := config.SubstituteVars(nodeCopy, layerVars); err != nil {
			return nil, fmt.Errorf("layer %s: %w", layerPath, err)
		}

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
				return nil, fmt.Errorf("layer %s (%s): resolving source: %w", layerPath, step.Action, err)
			}
			if step.LayerScript != "" || step.LayerScriptPath != "" {
				if err := runLayerScript(step, layerDir, sourceDir); err != nil {
					return nil, fmt.Errorf("layer %s (%s): layer script: %w", layerPath, step.Action, err)
				}
			}
			effectiveLayerDir = sourceDir
		}

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

	return layerVars, nil
}

// propagateVars copies variable values from layerVars back into the outer vars
// map. If exports is non-empty, only the listed names are propagated.
func propagateVars(exports []string, layerVars, vars map[string]string) {
	if len(exports) > 0 {
		for _, name := range exports {
			if v, ok := layerVars[name]; ok {
				vars[name] = v
			}
		}
	} else {
		for k, v := range layerVars {
			vars[k] = v
		}
	}
}

// deduplicatePackages removes duplicate package entries, keeping the last
// occurrence (later layers win and can override/pin versions).
func deduplicatePackages(pkgs []actions.Package) []actions.Package {
	if len(pkgs) == 0 {
		return pkgs
	}
	seen := make(map[string]int)
	var unique []actions.Package
	for _, pkg := range pkgs {
		if idx, ok := seen[pkg.Name]; ok {
			unique[idx] = pkg
		} else {
			seen[pkg.Name] = len(unique)
			unique = append(unique, pkg)
		}
	}
	return unique
}

// resolvePkgRels resolves the pkgrel for any pinned packages that specify only
// a version without a pkgrel suffix (e.g. "1.0" → "1.0-2"). This ensures the
// phase hash reflects the exact package that will be installed.
func (b *Builder) resolvePkgRels(ctx *actions.BuildContext, verbose bool) error {
	if b.DryRun {
		return nil
	}
	for i, pkg := range ctx.Packages {
		if pkg.Version == "" || strings.Contains(pkg.Version, "-") {
			continue
		}
		resolved, err := resolveLatestPkgrel(pkg.Name, pkg.Version)
		if err != nil {
			return fmt.Errorf("resolving pkgrel for %s=%s: %w", pkg.Name, pkg.Version, err)
		}
		if verbose {
			out.Info("resolved %s=%s → %s=%s", pkg.Name, pkg.Version, pkg.Name, resolved)
		}
		ctx.Packages[i].Version = resolved
	}
	return nil
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
		out.Styled(
			fmt.Sprintf("        %s %s %s", stepStyle.Render(step.Action), dimStyle.Render("›"), dimStyle.Render(step.Label)),
			fmt.Sprintf("        %s › %s", step.Action, step.Label),
		)
	} else {
		out.Styled(
			fmt.Sprintf("        %s", stepStyle.Render(step.Action)),
			fmt.Sprintf("        %s", step.Action),
		)
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
	out.StartStage(StageBuild)
	buildStart := time.Now()

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
		out.Info("Cache version mismatch (have %d, need %d), cleaning...", manifest.Version, CacheVersion)
		if err := overlay.CleanCache(); err != nil {
			return fmt.Errorf("cleaning incompatible cache: %w", err)
		}
		if err := overlay.Init(); err != nil {
			return fmt.Errorf("reinitializing overlay: %w", err)
		}
		manifest = &Manifest{Version: CacheVersion, Phases: make(map[string]PhaseEntry)}
		out.Blank()
	}

	// Set version for new caches
	manifest.Version = CacheVersion

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
			out.PhaseCached(phaseName)
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

		out.Phase(phaseName)
		phaseStart := time.Now()

		// Execute the phase into the merged directory
		if err := phaseFn(ctx, overlay.MergedDir()); err != nil {
			out.PhaseFailed(phaseName)
			overlay.Unmount()
			return fmt.Errorf("phase %s: %w", phaseName, err)
		}

		out.PhaseComplete(phaseName, time.Since(phaseStart))

		// Commit: unmount and save hash
		if err := overlay.CommitPhase(i, hash, manifest); err != nil {
			return fmt.Errorf("committing phase %s: %w", phaseName, err)
		}
	}

	out.Blank()

	out.EndStage(StageBuild, time.Since(buildStart))
	return nil
}
