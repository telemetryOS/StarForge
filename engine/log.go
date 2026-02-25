package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// startBuildLog tees all stdout/stderr output to a build.log file in buildDir.
// It replaces os.Stdout and os.Stderr with pipe write-ends; goroutines read
// from the pipe read-ends and write to both the original terminal fd and the
// log file. The returned cleanup function restores the original file descriptors,
// waits for goroutines to drain, and closes the log file.
//
// If log creation fails, returns a nil cleanup and the error — callers should
// treat this as non-fatal (warn and continue without logging).
func startBuildLog(buildDir string) (cleanup func(), err error) {
	logPath := filepath.Join(buildDir, "build.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("creating build.log: %w", err)
	}

	// Write timestamp header
	fmt.Fprintf(logFile, "Build started: %s\n\n", time.Now().Format(time.RFC3339))

	// Save originals
	origStdout := os.Stdout
	origStderr := os.Stderr

	// Create os.Pipe pairs (real fds so child processes inherit them)
	outR, outW, err := os.Pipe()
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		logFile.Close()
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	// Replace globals
	os.Stdout = outW
	os.Stderr = errW

	// Mutex protects logFile writes from concurrent stdout/stderr goroutines
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	tee := func(r *os.File, orig *os.File) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				orig.Write(chunk)
				mu.Lock()
				logFile.Write(chunk)
				mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}

	go tee(outR, origStdout)
	go tee(errR, origStderr)

	return func() {
		// Close write-ends so tee goroutines see EOF
		outW.Close()
		errW.Close()

		// Wait for goroutines to drain all buffered data
		wg.Wait()

		// Close read-ends
		outR.Close()
		errR.Close()

		// Restore original fds
		os.Stdout = origStdout
		os.Stderr = origStderr

		// Write footer and close log
		fmt.Fprintf(logFile, "\nBuild ended: %s\n", time.Now().Format(time.RFC3339))
		logFile.Close()
	}, nil
}

// wrapBuildLog is a convenience that calls startBuildLog and handles the
// non-fatal error case: if logging cannot be set up, it prints a warning
// and returns a no-op cleanup. Callers can unconditionally defer the
// returned function.
func wrapBuildLog(buildDir string) func() {
	cleanup, err := startBuildLog(buildDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: build log disabled: %v\n", err)
		return func() {}
	}
	return cleanup
}

