package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

func (b *Builder) phaseFiles(ctx *actions.BuildContext, rootfs string) error {
	// 1. Create directories (file-mkdir)
	for _, m := range ctx.FileMkdirs {
		target := filepath.Join(rootfs, m.Path)
		out.Info("mkdir %s%s", m.Path, labelSuffix(m.Label))
		mode, err := parseMode(m.Mode, 0o755)
		if err != nil {
			return fmt.Errorf("mkdir %s: %w", m.Path, err)
		}
		if err := mkdirAllInherit(target, mode); err != nil {
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
		src := cp.FromPath
		if !filepath.IsAbs(src) {
			src = filepath.Join(cp.LayerDir, src)
		}
		dest := filepath.Join(rootfs, cp.ToPath)
		out.Info("%s -> %s%s", cp.FromPath, cp.ToPath, labelSuffix(cp.Label))

		srcInfo, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", cp.FromPath, err)
		}

		if srcInfo.IsDir() {
			if err := mkdirAllInherit(dest, 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", cp.ToPath, err)
			}
		} else {
			if err := mkdirAllInherit(filepath.Dir(dest), 0o755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", cp.ToPath, err)
			}
		}

		fs := filesystemForPath(cp.ToPath, ctx.Partitions)
		if err := copyForFilesystem(src, dest, fs); err != nil {
			return fmt.Errorf("copying %s to %s: %w", cp.FromPath, cp.ToPath, err)
		}
		inheritOwnership(dest)
	}

	// 3. File creates (file-create with content or file layer_path)
	for _, fc := range ctx.FileCreates {
		out.Info("create %s%s", fc.Path, labelSuffix(fc.Label))
		target := filepath.Join(rootfs, fc.Path)

		if err := mkdirAllInherit(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", fc.Path, err)
		}
		mode, err := parseMode(fc.Mode, 0o644)
		if err != nil {
			return fmt.Errorf("file-create %s: %w", fc.Path, err)
		}
		if err := os.WriteFile(target, []byte(fc.Content), mode); err != nil {
			return fmt.Errorf("writing %s: %w", fc.Path, err)
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", fc.Path, err)
		}
		inheritOwnership(target)
	}

	// 4. File edits (file-edit)
	for _, fe := range ctx.FileEdits {
		out.Info("edit %s%s", fe.Path, labelSuffix(fe.Label))
		if err := applyFileEdit(rootfs, fe); err != nil {
			return err
		}
	}

	// 5. Internal copies (file-copy, within target)
	for _, ic := range ctx.FileCopies {
		out.Info("copy %s -> %s%s", ic.FromPath, ic.ToPath, labelSuffix(ic.Label))
		src := filepath.Join(rootfs, ic.FromPath)
		dest := filepath.Join(rootfs, ic.ToPath)
		if err := mkdirAllInherit(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", ic.ToPath, err)
		}
		if err := run("cp", "-rT", src, dest); err != nil {
			return fmt.Errorf("copying %s to %s: %w", ic.FromPath, ic.ToPath, err)
		}
		inheritOwnership(dest)
	}

	// 6. Moves (file-move)
	for _, mv := range ctx.FileMoves {
		out.Info("move %s -> %s%s", mv.FromPath, mv.ToPath, labelSuffix(mv.Label))
		src := filepath.Join(rootfs, mv.FromPath)
		dest := filepath.Join(rootfs, mv.ToPath)
		if err := mkdirAllInherit(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", mv.ToPath, err)
		}
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("moving %s to %s: %w", mv.FromPath, mv.ToPath, err)
		}
		inheritOwnership(dest)
	}

	// 7. Links (file-link)
	for _, ln := range ctx.FileLinks {
		out.Info("%s %s -> %s%s", ln.Type, ln.ToPath, ln.FromPath, labelSuffix(ln.Label))
		dest := filepath.Join(rootfs, ln.ToPath)
		if err := mkdirAllInherit(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", ln.ToPath, err)
		}
		_ = os.Remove(dest) // remove existing link/file if present
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
		out.Info("delete %s%s", r.Path, labelSuffix(r.Label))
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
		result, err := actions.TruncatePattern(content, edit.Pattern, edit.Truncate, edit.Match)
		if err != nil {
			return fmt.Errorf("file-edit %s: %w", edit.Path, err)
		}
		content = result
	case edit.Insert == "append":
		content += edit.Content
	case edit.Insert == "prepend":
		content = edit.Content + content
	case edit.Insert == "before" || edit.Insert == "after":
		result, err := actions.InsertPattern(content, edit.Pattern, edit.Content, edit.Insert, edit.Match)
		if err != nil {
			return fmt.Errorf("file-edit %s: %w", edit.Path, err)
		}
		content = result
	}

	return os.WriteFile(path, []byte(content), mode)
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
