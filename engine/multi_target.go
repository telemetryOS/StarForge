package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

// PackageMultiTarget packages partition images for hostName, merging
// contributions from each transitive install-embed dependency.
//
// Shared partitions accept contributions from every target that mounts
// them. Targets are processed host-first, then embeds in dependency
// (post-order) order; later writers overwrite earlier ones at the same
// path. The engine does not arbitrate kernel-file conflicts on shared
// boot partitions — OS developers are expected to design their layers so
// shared paths don't collide (e.g. recovery uses linux-lts so its kernel
// filenames differ from device's linux).
//
// With zero embeds the flow degenerates to single-target packaging — only
// the host's overlay contributes, and only the host's pass runs the per-
// target packaging steps (fstab, ownership, bootctl install).
//
// hostMerged is the path to the host's mounted merged overlay.
// buildDir is the host's build directory (where partition images are
// created and the final disk is assembled).
//
// Returns the merged disk partition list using the host target's mount points
// so the caller can save it and use it for downstream operations like QEMU,
// device writes, and installer bundling.
func (b *Builder) PackageMultiTarget(
	hostName string,
	hostCtx *actions.BuildContext,
	hostMerged string,
	buildDir string,
	hostOps PackageOps,
) ([]actions.PartitionDef, error) {
	// Collect transitive embed deps (no-op when InstallEmbeds is empty).
	embeds, err := b.transitiveEmbeds(hostName)
	if err != nil {
		return nil, err
	}

	// Load each embed's saved BuildResult so we know its partition layout
	// and ownership/permission ops.
	embedResults := make(map[string]*BuildResult, len(embeds))
	for _, e := range embeds {
		r, err := LoadBuildResult(b.project.TargetBuildDir(e))
		if err != nil {
			return nil, fmt.Errorf("loading build result for embed %q: %w", e, err)
		}
		embedResults[e] = r
	}

	// Build the contribution list: host first, then embeds in dependency order.
	contribs := []actions.PartitionContribution{
		{Target: hostName, Parts: hostCtx.Partitions},
	}
	for _, e := range embeds {
		contribs = append(contribs, actions.PartitionContribution{
			Target: e,
			Parts:  embedResults[e].Partitions,
		})
	}

	merged, err := actions.MergePartitions(contribs)
	if err != nil {
		return nil, fmt.Errorf("merging partition tables: %w", err)
	}

	// Mount each embed's overlay so we can read its file contributions.
	overlays := map[string]string{hostName: hostMerged}
	var embedManagers []*OverlayManager
	defer func() {
		for _, om := range embedManagers {
			om.Unmount()
		}
	}()
	for _, e := range embeds {
		om := NewOverlayManager(b.project.TargetBuildDir(e))
		if err := om.Init(); err != nil {
			return nil, fmt.Errorf("initializing overlay for embed %q: %w", e, err)
		}
		mp, err := om.MountMerged()
		if err != nil {
			return nil, fmt.Errorf("mounting overlay for embed %q: %w", e, err)
		}
		overlays[e] = mp
		embedManagers = append(embedManagers, om)
	}

	flatParts := mergedDiskPartitions(hostName, merged)

	// Create + format partition images once in the host's build dir.
	if _, err := SetupImagePartitions(flatParts, buildDir); err != nil {
		return nil, fmt.Errorf("creating partition images: %w", err)
	}

	// Now process each target: mount its view of the partitions, copy its
	// overlay subtrees in, run per-target packaging steps. Host first so
	// bootctl install runs against a populated rootfs that has the bootloader
	// binaries installed via pacman; embeds afterward populate their own
	// rootfs partitions and may install-payload host's now-existing images.
	targets := append([]string{hostName}, embeds...)
	for _, t := range targets {
		isHost := t == hostName
		var (
			ops       PackageOps
			targetCtx *actions.BuildContext
		)
		if isHost {
			ops = hostOps
			targetCtx = hostCtx
		} else {
			ops = PackageOps{
				Ownerships:  embedResults[t].Ownerships,
				Permissions: embedResults[t].Permissions,
			}
			targetCtx = buildResultToContext(embedResults[t])
		}
		if err := b.packageOneTarget(t, isHost, merged, overlays[t], buildDir, ops, targetCtx); err != nil {
			return nil, fmt.Errorf("packaging target %q: %w", t, err)
		}
	}

	return flatParts, nil
}

// transitiveEmbeds returns every target reachable from hostName via the
// install-embed action, post-order, deduplicated. Empty slice if no embeds.
//
// Reads each target's BuildResult to discover its embeds — relies on the
// caller having run Build for the host (and transitively the embeds) first.
//
// Cycles are detected and reported as errors. buildRecursive's cycle
// detection prevents broken configs from reaching this far in normal flow,
// but a hand-edited or stale `build-result.json` could still describe a
// cycle, so we guard against runaway recursion.
func (b *Builder) transitiveEmbeds(hostName string) ([]string, error) {
	const (
		unvisited = 0
		onStack   = 1
		done      = 2
	)
	state := map[string]int{}
	var order []string

	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case onStack:
			cycle := append(append([]string(nil), path...), name)
			return fmt.Errorf("install-embed cycle: %s", strings.Join(cycle, " -> "))
		case done:
			return nil
		}
		state[name] = onStack
		path = append(path, name)
		r, err := LoadBuildResult(b.project.TargetBuildDir(name))
		if err != nil {
			return fmt.Errorf("loading build result for %q: %w", name, err)
		}
		for _, em := range r.InstallEmbeds {
			if err := visit(em, path); err != nil {
				return err
			}
			if state[em] == done {
				// Append in post-order, deduplicated.
				alreadyEmitted := false
				for _, o := range order {
					if o == em {
						alreadyEmitted = true
						break
					}
				}
				if !alreadyEmitted {
					order = append(order, em)
				}
			}
		}
		state[name] = done
		return nil
	}

	if err := visit(hostName, nil); err != nil {
		return nil, err
	}
	return order, nil
}

// packageOneTarget handles the per-target portion of multi-target packaging:
// mount the target's view of merged partitions at a staging rootfs, copy
// in its overlay subtrees, run fstab + ownership + (host) bootctl install,
// and bundle any install-* actions declared by this target.
func (b *Builder) packageOneTarget(
	targetName string,
	isHost bool,
	merged []actions.MergedPartition,
	overlayDir string,
	buildDir string,
	ops PackageOps,
	targetCtx *actions.BuildContext,
) error {
	// Build this target's view of the merged partitions.
	var mounts []PartitionMount
	var perTargetParts []actions.PartitionDef
	for _, mp := range merged {
		mountPoint, ok := mp.Mounts[targetName]
		if !ok {
			continue
		}
		imgPath := filepath.Join(buildDir, fmt.Sprintf("%s.img", mp.Name))
		mounts = append(mounts, PartitionMount{
			Source:     imgPath,
			MountPoint: mountPoint,
			Loop:       true,
		})
		perTargetParts = append(perTargetParts, actions.PartitionDef{
			Name:       mp.Name,
			Filesystem: mp.Filesystem,
			Size:       mp.Size,
			Type:       mp.Type,
			Grow:       mp.Grow,
			MountPoint: mountPoint,
		})
	}

	// Per-target staging rootfs. Sanitized name keeps the path predictable
	// and inside buildDir. Cleaned up after the pass so stale mountpoint
	// shells don't accumulate in .starforge/<host>/.
	rootfs := filepath.Join(buildDir, "rootfs-"+sanitizeTargetName(targetName))
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		return fmt.Errorf("creating staging rootfs: %w", err)
	}
	defer os.RemoveAll(rootfs)
	CleanupMounts(rootfs)

	mt := NewMountTable(rootfs)

	out.Blank()
	out.Phase(fmt.Sprintf("Mounting %s partitions", targetName))
	if err := mt.MountAll(mounts); err != nil {
		mt.Unmount()
		return fmt.Errorf("mounting partitions: %w", err)
	}
	defer func() {
		out.Blank()
		out.Phase(fmt.Sprintf("Unmounting %s partitions", targetName))
		mt.Unmount()
	}()

	// Copy this target's overlay subtrees into the mounted partitions.
	out.Blank()
	out.Phase(fmt.Sprintf("Copying %s contributions", targetName))
	for _, part := range perTargetParts {
		if err := CopyPartition(overlayDir, part, perTargetParts, rootfs); err != nil {
			return fmt.Errorf("copying %s: %w", part.MountPoint, err)
		}
	}

	if err := EnsureChrootDirs(rootfs); err != nil {
		return err
	}

	// Only the host runs bootctl install (one bootloader binary on the ESP).
	// Embeds' loader entries are already on the boot partition image from
	// the file copy above.
	if isHost {
		if err := InstallSystemdBoot(perTargetParts, rootfs); err != nil {
			return err
		}
	}

	// Each target patches its OWN entries with its OWN / partition's UUID.
	// Per-target isolation: recovery's entries get recovery's UUID,
	// fallback-recovery's entries get fallback-recovery's UUID, host's
	// entries get host's. No cross-target inference.
	if targetCtx != nil && targetCtx.Boot != nil && len(targetCtx.Boot.Entries) > 0 {
		if err := PatchBootEntries(perTargetParts, targetCtx.Boot.Entries, rootfs); err != nil {
			return err
		}
	}

	out.Blank()
	out.Phase(fmt.Sprintf("Generating fstab for %s", targetName))
	if err := GenerateFstab(perTargetParts, rootfs); err != nil {
		return err
	}

	if err := applyImageOwnership(rootfs, ops); err != nil {
		return err
	}

	// Bundle this target's install-* actions into its own rootfs while
	// it's still mounted. For the host this replaces the standalone
	// bundleInstaller call that EnsurePackaged used to make. For embeds
	// this is what lets the recovery rootfs ship the active boot/root
	// images for later flashing.
	if targetCtx != nil && HasInstallerActions(targetCtx) {
		if err := b.BundleInstallerToRootfs(targetCtx, rootfs); err != nil {
			return fmt.Errorf("bundling installer artifacts: %w", err)
		}
	}

	return nil
}

// sanitizeTargetName returns a target name safe to use as a single path
// component. Mirrors the safety behavior of project.TargetBuildDir.
func sanitizeTargetName(name string) string {
	clean := filepath.Base(filepath.Clean(name))
	if clean == "." || clean == ".." || clean == "" {
		return "_"
	}
	// Replace any leftover separators (defensive).
	return strings.ReplaceAll(clean, string(filepath.Separator), "_")
}

func mergedDiskPartitions(hostName string, merged []actions.MergedPartition) []actions.PartitionDef {
	parts := make([]actions.PartitionDef, 0, len(merged))
	for _, mp := range merged {
		parts = append(parts, actions.PartitionDef{
			Name:       mp.Name,
			Filesystem: mp.Filesystem,
			Size:       mp.Size,
			Type:       mp.Type,
			Grow:       mp.Grow,
			MountPoint: mp.Mounts[hostName],
		})
	}
	return parts
}
