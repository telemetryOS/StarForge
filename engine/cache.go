package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/telemetryos/starforge/actions"
)

// PhaseNames lists the build phases in execution order.
var PhaseNames = []string{
	"0-preinstall",
	"1-packages",
	"2-sysconfig",
	"3-users",
	"4-files",
	"5-permissions",
	"6-services",
	"7-boot",
	"8-scripts",
}

// CacheVersion is incremented when the overlay mount options or cache
// format changes. Caches with a different version are automatically cleaned.
const CacheVersion = 1

// Manifest tracks the input hash and completion status of each cached phase.
type Manifest struct {
	Version int                   `json:"version,omitempty"`
	Phases  map[string]PhaseEntry `json:"phases"`
}

// PhaseEntry records a single phase's cache state.
type PhaseEntry struct {
	Hash      string `json:"hash"`
	Completed bool   `json:"completed"`
}

// LoadManifest reads manifest.json from the cache directory.
// Returns an empty manifest if the file does not exist.
func LoadManifest(cacheDir string) (*Manifest, error) {
	path := filepath.Join(cacheDir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Phases: make(map[string]PhaseEntry)}, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if m.Phases == nil {
		m.Phases = make(map[string]PhaseEntry)
	}
	return &m, nil
}

// Save writes the manifest to manifest.json in the cache directory.
func (m *Manifest) Save(cacheDir string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	path := filepath.Join(cacheDir, "manifest.json")
	return os.WriteFile(path, data, 0o644)
}

// HashPhase computes a deterministic hash of the BuildContext fields
// that the given phase depends on.
func HashPhase(phaseIndex int, ctx *actions.BuildContext) (string, error) {
	h := sha256.New()

	switch phaseIndex {
	case 0: // preinstall
		fmt.Fprintf(h, "keymap=%s\n", ctx.Keymap)

	case 1: // packages
		pkgs := make([]string, len(ctx.Packages))
		copy(pkgs, ctx.Packages)
		sort.Strings(pkgs)
		fmt.Fprintf(h, "packages=%s\n", strings.Join(pkgs, ","))

	case 2: // sysconfig
		fmt.Fprintf(h, "hostname=%s\n", ctx.Hostname)
		fmt.Fprintf(h, "locale=%s\n", ctx.Locale)
		fmt.Fprintf(h, "timezone=%s\n", ctx.Timezone)
		fmt.Fprintf(h, "keymap=%s\n", ctx.Keymap)
		locales := make([]string, len(ctx.Locales))
		copy(locales, ctx.Locales)
		sort.Strings(locales)
		fmt.Fprintf(h, "locales=%s\n", strings.Join(locales, ","))

	case 3: // users
		for _, g := range ctx.Groups {
			fmt.Fprintf(h, "group=%s,gid=%d,system=%v\n", g.Name, g.GID, g.System)
		}
		for _, u := range ctx.Users {
			groups := make([]string, len(u.Groups))
			copy(groups, u.Groups)
			sort.Strings(groups)
			fmt.Fprintf(h, "user=%s,groups=%s,shell=%s,uid=%d,system=%v,nopassword=%v\n",
				u.Name, strings.Join(groups, "+"), u.Shell, u.UID, u.System, u.NoPassword)
			if u.Password != "" {
				fmt.Fprintf(h, "password=%x\n", sha256.Sum256([]byte(u.Password)))
			}
		}

	case 4: // files
		for _, m := range ctx.FileMkdirs {
			fmt.Fprintf(h, "mkdir=%s,mode=%s,owner=%s,group=%s\n", m.Path, m.Mode, m.Owner, m.Group)
		}
		for _, cp := range ctx.LayerCopies {
			fmt.Fprintf(h, "copy=%s->%s\n", cp.FromPath, cp.ToPath)
			srcPath := cp.FromPath
			if !filepath.IsAbs(srcPath) {
				srcPath = filepath.Join(cp.LayerDir, srcPath)
			}
			fileHash, err := hashPath(srcPath)
			if err != nil {
				return "", fmt.Errorf("hashing source %s: %w", srcPath, err)
			}
			fmt.Fprintf(h, "content=%s\n", fileHash)
		}
		for _, fc := range ctx.FileCreates {
			fmt.Fprintf(h, "create=%s,mode=%s\n", fc.Path, fc.Mode)
			fmt.Fprintf(h, "content=%x\n", sha256.Sum256([]byte(fc.Content)))
		}
		for _, fe := range ctx.FileEdits {
			fmt.Fprintf(h, "edit=%s,insert=%s,truncate=%s,pattern=%s,match=%d\n",
				fe.Path, fe.Insert, fe.Truncate, fe.Pattern, fe.Match)
			if fe.Content != "" {
				fmt.Fprintf(h, "econtent=%x\n", sha256.Sum256([]byte(fe.Content)))
			}
		}
		for _, ic := range ctx.FileCopies {
			fmt.Fprintf(h, "icopy=%s->%s\n", ic.FromPath, ic.ToPath)
		}
		for _, mv := range ctx.FileMoves {
			fmt.Fprintf(h, "move=%s->%s\n", mv.FromPath, mv.ToPath)
		}
		for _, ln := range ctx.FileLinks {
			fmt.Fprintf(h, "link=%s->%s,type=%s\n", ln.ToPath, ln.FromPath, ln.Type)
		}
		for _, r := range ctx.FileDeletes {
			fmt.Fprintf(h, "remove=%s,recursive=%v\n", r.Path, r.Recursive)
		}

	case 5: // permissions
		for _, o := range ctx.FileOwnerships {
			fmt.Fprintf(h, "own=%s,owner=%s,group=%s,recursive=%v\n",
				o.Path, o.Owner, o.Group, o.Recursive)
		}
		for _, p := range ctx.FilePermissions {
			fmt.Fprintf(h, "perm=%s,mode=%s,recursive=%v\n",
				p.Path, p.Mode, p.Recursive)
		}

	case 6: // services
		enable := make([]string, len(ctx.Services.Enable))
		copy(enable, ctx.Services.Enable)
		sort.Strings(enable)
		disable := make([]string, len(ctx.Services.Disable))
		copy(disable, ctx.Services.Disable)
		sort.Strings(disable)
		mask := make([]string, len(ctx.Services.Mask))
		copy(mask, ctx.Services.Mask)
		sort.Strings(mask)
		fmt.Fprintf(h, "enable=%s\n", strings.Join(enable, ","))
		fmt.Fprintf(h, "disable=%s\n", strings.Join(disable, ","))
		fmt.Fprintf(h, "mask=%s\n", strings.Join(mask, ","))
		fmt.Fprintf(h, "default-target=%s\n", ctx.DefaultTarget)
		// User-level service operations
		var userEnable, userDisable []string
		for _, op := range ctx.Services.UserEnable {
			userEnable = append(userEnable, op.User+":"+op.Service)
		}
		for _, op := range ctx.Services.UserDisable {
			userDisable = append(userDisable, op.User+":"+op.Service)
		}
		sort.Strings(userEnable)
		sort.Strings(userDisable)
		fmt.Fprintf(h, "user-enable=%s\n", strings.Join(userEnable, ","))
		fmt.Fprintf(h, "user-disable=%s\n", strings.Join(userDisable, ","))

	case 7: // boot
		if ctx.Boot != nil {
			fmt.Fprintf(h, "loader=%s,%d,%v\n",
				ctx.Boot.Loader.Default, ctx.Boot.Loader.Timeout, ctx.Boot.Loader.Editor)
			for _, e := range ctx.Boot.Entries {
				fmt.Fprintf(h, "entry=%s,%s,%s,%s,%s\n",
					e.Name, e.Title, e.Linux, e.Initrd, e.Options)
			}
		}

	case 8: // scripts
		// Include target-level env in hash
		if len(ctx.Env) > 0 {
			envKeys := make([]string, 0, len(ctx.Env))
			for k := range ctx.Env {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			for _, k := range envKeys {
				fmt.Fprintf(h, "env=%s=%s\n", k, ctx.Env[k])
			}
		}
		// Include resolved vars in hash
		if len(ctx.Vars) > 0 {
			varKeys := make([]string, 0, len(ctx.Vars))
			for k := range ctx.Vars {
				varKeys = append(varKeys, k)
			}
			sort.Strings(varKeys)
			for _, k := range varKeys {
				fmt.Fprintf(h, "var=%s=%s\n", k, ctx.Vars[k])
			}
		}
		for _, s := range ctx.Scripts {
			if s.Script != "" {
				fmt.Fprintf(h, "script=%s\n", s.Script)
				scriptPath := filepath.Join(s.LayerDir, s.Script)
				fileHash, err := hashPath(scriptPath)
				if err != nil {
					return "", fmt.Errorf("hashing script %s: %w", scriptPath, err)
				}
				fmt.Fprintf(h, "content=%s\n", fileHash)
			} else {
				fmt.Fprintf(h, "inline=%s\n", s.Content)
			}
			if s.User != "" {
				fmt.Fprintf(h, "user=%s\n", s.User)
			}
			// Include step-level env in hash
			if len(s.Env) > 0 {
				sEnvKeys := make([]string, 0, len(s.Env))
				for k := range s.Env {
					sEnvKeys = append(sEnvKeys, k)
				}
				sort.Strings(sEnvKeys)
				for _, k := range sEnvKeys {
					fmt.Fprintf(h, "senv=%s=%s\n", k, s.Env[k])
				}
			}
		}

	default:
		return "", fmt.Errorf("unknown phase index: %d", phaseIndex)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// InvalidateFrom deletes cached overlay layers from phaseIndex onward.
func InvalidateFrom(cacheDir string, phaseIndex int, manifest *Manifest) error {
	for i := phaseIndex; i < len(PhaseNames); i++ {
		name := PhaseNames[i]
		delete(manifest.Phases, name)

		layerDir := filepath.Join(cacheDir, name)
		if err := os.RemoveAll(layerDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing cached layer %s: %w", name, err)
		}
	}
	return nil
}

// IsPhaseCached returns true if the phase has a matching hash and completed layer.
func IsPhaseCached(cacheDir string, phaseIndex int, hash string, manifest *Manifest) bool {
	name := PhaseNames[phaseIndex]
	entry, ok := manifest.Phases[name]
	if !ok || !entry.Completed || entry.Hash != hash {
		return false
	}

	// Verify the upper directory actually exists
	upperDir := filepath.Join(cacheDir, name, "upper")
	if _, err := os.Stat(upperDir); os.IsNotExist(err) {
		return false
	}
	return true
}

// hashPath computes the sha256 of a file or directory tree.
func hashPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}

	h := sha256.New()

	if !info.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	// For directories, walk and hash all file contents + relative paths
	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(path, p)
		fmt.Fprintf(h, "path=%s,mode=%o,size=%d\n", rel, fi.Mode(), fi.Size())

		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "link=%s\n", target)
			return nil
		}

		if !fi.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(h, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

