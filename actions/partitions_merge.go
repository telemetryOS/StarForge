package actions

import "fmt"

// GPT partition-type aliases used by the engine to look up partitions by
// role. Matches the values accepted by isValidPartitionType in resolve.go
// and sfdiskTypeAlias in engine/qemu.go.
const (
	PartTypeEFI      = "efi"
	PartTypeXBOOTLDR = "xbootldr"
	PartTypeLinux    = "linux"
)

// MergedPartition is one physical partition on a disk that may be mounted at
// different paths in different targets' rootfs trees.
type MergedPartition struct {
	Name       string
	Filesystem string
	Size       uint64
	Type       string
	Grow       bool

	// Mounts maps target name → mount point in that target's rootfs.
	// A target not present in this map does not mount this partition,
	// and its overlay tree will not contribute files to the partition's image.
	Mounts map[string]string
}

// PartitionContribution is one target's set of partition declarations.
type PartitionContribution struct {
	Target string
	Parts  []PartitionDef
}

// MergePartitions combines partition declarations from a host target and any
// number of embedded targets into a single ordered partition table.
//
// contribs[0] is treated as the host. The remaining entries are embeds, in the
// order they appear in the host's `embed:` list.
//
// Merge rules for a partition that appears under the same name in more than
// one contribution:
//   - filesystem must be identical (else error)
//   - type must be identical (else error)
//   - size: the merged size is the maximum across all contributions; the host's
//     declared size (if it declares this partition) must not be smaller than
//     any embed's declared size (else error)
//   - grow: if any contribution requests grow, the merged partition grows
//
// Mount points are kept per-target via MergedPartition.Mounts so the same
// physical partition can mount at different paths in each rootfs's fstab.
//
// The merged list is ordered host-first, then the order in which new partition
// names are first introduced by successive embeds.
func MergePartitions(contribs []PartitionContribution) ([]MergedPartition, error) {
	if len(contribs) == 0 {
		return nil, nil
	}

	// Reject duplicate target names — silently overwriting Mounts entries
	// across two contribs sharing a name would corrupt the per-target view.
	seenTarget := map[string]bool{}
	for i, c := range contribs {
		if c.Target == "" {
			return nil, fmt.Errorf("contribution %d has empty target name", i)
		}
		if seenTarget[c.Target] {
			return nil, fmt.Errorf("duplicate target name %q in partition contributions", c.Target)
		}
		seenTarget[c.Target] = true
	}

	merged := []MergedPartition{}
	idxByName := map[string]int{}

	// Pass 1: build the merged table, validate filesystem/type, take max size.
	// The host-size rule is enforced separately in pass 2.
	for _, c := range contribs {
		for _, p := range c.Parts {
			if p.Filesystem == "" {
				return nil, fmt.Errorf("target %q: partition %q has empty filesystem", c.Target, p.Name)
			}
			if p.Type == "" {
				return nil, fmt.Errorf("target %q: partition %q has empty type", c.Target, p.Name)
			}
			if p.Name == "" {
				return nil, fmt.Errorf("target %q: partition has empty name", c.Target)
			}
			if idx, ok := idxByName[p.Name]; ok {
				m := &merged[idx]
				if m.Filesystem != p.Filesystem {
					return nil, fmt.Errorf("partition %q: filesystem mismatch: %q declares %q, earlier declaration is %q",
						p.Name, c.Target, p.Filesystem, m.Filesystem)
				}
				if m.Type != p.Type {
					return nil, fmt.Errorf("partition %q: type mismatch: %q declares %q, earlier declaration is %q",
						p.Name, c.Target, p.Type, m.Type)
				}
				if _, dup := m.Mounts[c.Target]; dup {
					return nil, fmt.Errorf("target %q declares partition %q more than once", c.Target, p.Name)
				}
				if p.Size > m.Size {
					m.Size = p.Size
				}
				if p.Grow {
					m.Grow = true
				}
				m.Mounts[c.Target] = p.MountPoint
			} else {
				merged = append(merged, MergedPartition{
					Name:       p.Name,
					Filesystem: p.Filesystem,
					Size:       p.Size,
					Type:       p.Type,
					Grow:       p.Grow,
					Mounts:     map[string]string{c.Target: p.MountPoint},
				})
				idxByName[p.Name] = len(merged) - 1
			}
		}
	}

	// Pass 2: enforce the host-size rule. If the host declares a partition,
	// its declared size must be >= every embed's declared size for that name.
	if len(contribs) > 1 {
		host := contribs[0]
		hostSizes := map[string]uint64{}
		hostDeclares := map[string]bool{}
		for _, p := range host.Parts {
			hostSizes[p.Name] = p.Size
			hostDeclares[p.Name] = true
		}
		for _, c := range contribs[1:] {
			for _, p := range c.Parts {
				if !hostDeclares[p.Name] {
					continue
				}
				if p.Size > hostSizes[p.Name] {
					return nil, fmt.Errorf(
						"partition %q: embed %q requires size %d but host %q declares only %d",
						p.Name, c.Target, p.Size, host.Target, hostSizes[p.Name])
				}
			}
		}
	}

	return merged, nil
}
