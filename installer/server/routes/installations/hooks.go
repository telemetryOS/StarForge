package installations

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// hooksRoot is the directory under which lifecycle hook subdirectories live.
// Overridable for testing. In production it points at the running installer
// USB's filesystem, where Edge-OS layers drop scripts via file-create.
var hooksRoot = "/usr/lib/starforge/hooks"

// runInstallHooks executes every regular executable under
// <hooksRoot>/<phase>.d/ in lexical order, passing (targetRootfs, payloadDir)
// as arguments. Output streams into the installation log via inst.addLog.
//
// Returns nil and is a no-op if the phase directory doesn't exist or contains
// no executables. A non-zero exit from any script aborts the loop and is
// returned as an error; subsequent scripts are not invoked.
//
// On-failure callers (which are themselves reacting to an error) should
// log but not propagate any error this returns — the caller is already
// failing, and a misbehaving cleanup hook shouldn't compound the message.
func runInstallHooks(phase, targetRootfs, payloadDir string, inst *Installation) error {
	dir := filepath.Join(hooksRoot, phase+".d")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading hooks dir %s: %w", dir, err)
	}

	// Lexical order, by filename.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		// Regular file that's executable by anyone.
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}

		inst.addLog(fmt.Sprintf("hook %s/%s", phase, e.Name()))
		if err := runOneHook(path, targetRootfs, payloadDir, inst); err != nil {
			return fmt.Errorf("hook %s: %w", e.Name(), err)
		}
	}
	return nil
}

// runOneHook executes a single hook script, plumbing its stdout/stderr into
// the installation log line-by-line. Returns an error if the script exits
// with non-zero status.
func runOneHook(scriptPath, targetRootfs, payloadDir string, inst *Installation) error {
	cmd := exec.Command(scriptPath, targetRootfs, payloadDir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// Stream both streams concurrently into the installation log. Both
	// streams must be drained before Wait or the child can deadlock.
	done := make(chan struct{}, 2)
	streamLines := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			inst.addLog(scanner.Text())
		}
		done <- struct{}{}
	}
	go streamLines(stdout)
	go streamLines(stderr)
	<-done
	<-done

	return cmd.Wait()
}
